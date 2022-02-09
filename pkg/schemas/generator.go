// Copyright 2021 IBM Corp.
// SPDX-License-Identifier: Apache-2.0

package schemas

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"log"
	"os"
	"path/filepath"
	"strings"

	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-tools/pkg/crd"
	crdmarkers "sigs.k8s.io/controller-tools/pkg/crd/markers"
	"sigs.k8s.io/controller-tools/pkg/genall"
	"sigs.k8s.io/controller-tools/pkg/loader"
	"sigs.k8s.io/controller-tools/pkg/markers"
)

var (
	externalDocumentName = "external.json"
	schemaMarker         = markers.Must(markers.MakeDefinition("fybrik:validation:schema", markers.DescribesPackage, struct{}{}))
	objectMarker         = markers.Must(markers.MakeDefinition("fybrik:validation:object", markers.DescribesType, struct{}{}))
)

// Generator generates JSON schema objects.
type Generator struct {
	OutputDir string

	// AllowDangerousTypes allows types which are usually omitted from CRD generation
	// because they are not recommended.
	//
	// Currently the following additional types are allowed when this is true:
	// float32
	// float64
	//
	// Left unspecified, the default is false
	AllowDangerousTypes *bool `marker:",optional"`
}

type GeneratorContext struct {
	ctx        *genall.GenerationContext
	parser     *crd.Parser
	pkgMarkers map[*loader.Package]markers.MarkerValues
}

func (Generator) CheckFilter() loader.NodeFilter {
	return func(node ast.Node) bool {
		// ignore interfaces
		// _, isIface := node.(*ast.InterfaceType)
		return true
	}
}

func (Generator) RegisterMarkers(into *markers.Registry) error {
	// TODO: only register validation markers
	if err := crdmarkers.Register(into); err != nil {
		return err
	}

	if err := markers.RegisterAll(into, schemaMarker, objectMarker); err != nil {
		return err
	}
	into.AddHelp(schemaMarker,
		markers.SimpleHelp("object", "enable generation of JSON schema definition for the go structure"))
	into.AddHelp(objectMarker,
		markers.SimpleHelp("object", "enable generation of JSON schema object for the go structure"))
	return nil
}

func (g Generator) Generate(ctx *genall.GenerationContext) error {
	fmt.Println("generate******")
	parser := &crd.Parser{
		Collector:           ctx.Collector,
		Checker:             ctx.Checker,
		AllowDangerousTypes: g.AllowDangerousTypes != nil && *g.AllowDangerousTypes,
	}
	crd.AddKnownTypes(parser)
	for _, root := range ctx.Roots {
		parser.NeedPackage(root)
	}

	/////////////////////
	// metav1Pkg := crd.FindMetav1(ctx.Roots)
	// if metav1Pkg == nil {
	// 	// no objects in the roots, since nothing imported metav1
	// 	return nil
	// }

	// // TODO: allow selecting a specific object
	// kubeKinds := crd.FindKubeKinds(parser, metav1Pkg)
	// if len(kubeKinds) == 0 {
	// 	// no objects in the roots
	// 	return nil
	// }
	// for groupKind := range kubeKinds {
	// 	m := 100
	// 	parser.NeedCRDFor(groupKind, &m)
	// 	crdRaw := parser.CustomResourceDefinitions[groupKind]
	// 	// crd.addAttribution(&crdRaw)

	// 	// Prevent the top level metadata for the CRD to be generate regardless of the intention in the arguments
	// 	crd.FixTopLevelMetadata(crdRaw)
	// 	// fmt.Printf("group kind = %s, crdRaw = %s\n", groupKind, parser.CustomResourceDefinitions[groupKind])

	// }
	/////////////////////

	context := &GeneratorContext{
		ctx:    ctx,
		parser: parser,
		// documents:  make(map[string]*apiext.JSONSchemaProps),
		pkgMarkers: make(map[*loader.Package]markers.MarkerValues),
	}

	for typeIdent := range parser.Types {
		context.NeedSchemaFor(typeIdent)
	}

	documents := make(map[string]*apiext.JSONSchemaProps)
	//nolint:gocritic
	for typeIdent, typeSchema := range parser.Schemata {
		fmt.Printf("generate, type = %s, typeIdentSchema = %s, allof = %s\n", typeIdent, &typeSchema, typeSchema.AllOf)
		documentName := context.documentNameFor(typeIdent.Package)
		document, exists := documents[documentName]
		if !exists {
			document = &apiext.JSONSchemaProps{
				Title:       documentName,
				Definitions: make(apiext.JSONSchemaDefinitions),
			}
			documents[documentName] = document
		}
		document.Definitions[context.definitionNameFor(documentName, typeIdent)] = typeSchema

		// CRD
		info, knownInfo := parser.Types[typeIdent]
		if !knownInfo {
			fmt.Printf("unknown type %s", typeIdent.Name)
			// return g.output(documents)
		} else {

			if info.Markers.Get(objectMarker.Name) != nil {
				documentName := fmt.Sprintf("%s.json", info.Name)
				document, exists := documents[documentName]
				if !exists {
					fmt.Printf("inside if document %s\n", typeIdent.Name)
					document = &apiext.JSONSchemaProps{
						Title:       documentName,
						Definitions: make(apiext.JSONSchemaDefinitions),
					}
					documents[documentName] = document
				}
				removeMetadataProps(&typeSchema)
				document.Definitions[context.definitionNameFor(documentName, typeIdent)] = typeSchema

				listFields := context.GetFields(typeIdent)
				fmt.Printf("list fields %s\n", listFields)
				for _, fieldType := range listFields {
					typeSchemaField := parser.Schemata[fieldType]
					removeMetadataProps(&typeSchemaField)
					document.Definitions[context.definitionNameFor(documentName, fieldType)] = typeSchemaField
				}
			}
		}
	}

	return g.output(documents)
}

func (context *GeneratorContext) GetFields(typ crd.TypeIdent) []crd.TypeIdent {
	ListFields := []crd.TypeIdent{}
	info, knownInfo := context.parser.Types[typ]
	if !knownInfo {
		fmt.Printf("unknown type %s\n", typ.Name)
		return ListFields
	} else {
		fields := info.Fields
		for _, field := range fields {
			type_name := field.RawField.Type
			typeNameStr := fmt.Sprintf("%s", type_name)
			words := strings.Fields(typeNameStr)
			word := words[len(words)-1]
			if word[len(word)-1:] == "}" {
				word = word[:len(word)-1]
			}
			fmt.Printf("field type name %s, words = %s\n", type_name, word)
			typeIdentField := crd.TypeIdent{Package: typ.Package, Name: word}
			_, fieldKnownInfo := context.parser.Types[typeIdentField]
			if !fieldKnownInfo {
				// fmt.Printf("unknown type %s\n", typ.Name)
				continue
			}
			ListFields = append(ListFields, typeIdentField)
			ListFields = append(ListFields, context.GetFields(typeIdentField)...)
		}
	}
	// fmt.Printf("unknown type %s", typ.Name)

	return ListFields
}

func removeMetadataProps(v *apiext.JSONSchemaProps) {
	if _, ok := v.Properties["metadata"]; ok {
		delete(v.Properties, "metadata")
		v.AllOf = nil
		// meta := &m
		// if meta.Description != "" {
		// 	meta.Description = ""
		// 	v.Properties["metadata"] = ""

		// }
	}
}

func (g Generator) output(documents map[string]*apiext.JSONSchemaProps) error {
	// create out dir if needed
	err := os.MkdirAll(g.OutputDir, os.ModePerm)
	if err != nil {
		return err
	}

	for docName, doc := range documents {
		outputFilepath := filepath.Clean(filepath.Join(g.OutputDir, docName))

		// create the file
		f, err := os.Create(outputFilepath)
		if err != nil {
			return err
		}
		//nolint:gocritic
		defer func() {
			if err = f.Close(); err != nil {
				log.Printf("Error closing file: %s\n", err)
			}
		}()

		bytes, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return err
		}
		_, err = f.Write(bytes)
		if err != nil {
			return err
		}
	}

	return nil
}

func (context *GeneratorContext) documentNameFor(pkg *loader.Package) string {
	isManaged := context.pkgMarkers[pkg].Get(schemaMarker.Name) != nil
	isObjectMArk := context.pkgMarkers[pkg].Get(objectMarker.Name) != nil
	fmt.Printf("documentName, %s, object %t, schema %t\n", pkg.Name, isObjectMArk, isManaged)
	if isManaged {
		return fmt.Sprintf("%s.json", pkg.Name)
	}
	return externalDocumentName
}

func (context *GeneratorContext) definitionNameFor(documentName string, typeIdent crd.TypeIdent) string {
	if documentName == externalDocumentName {
		return qualifiedName(loader.NonVendorPath(typeIdent.Package.PkgPath), typeIdent.Name)
	}
	return typeIdent.Name
}

// qualifiedName constructs a JSONSchema-safe qualified name for a type
// (`<typeName>` or `<safePkgPath>~0<typeName>`, where `<safePkgPath>`
// is the package path with `/` replaced by `~1`, according to JSONPointer
// escapes).
func qualifiedName(pkgName, typeName string) string {
	if pkgName != "" {
		return strings.Replace(pkgName, "/", "~1", -1) + "~0" + typeName
	}
	return typeName
}

func (context *GeneratorContext) TypeRefLink(from *loader.Package, to crd.TypeIdent) string {
	fromDocument := context.documentNameFor(from)
	toDocument := context.documentNameFor(to.Package)

	prefix := "#/definitions/"
	if fromDocument != toDocument {
		prefix = toDocument + prefix
	}

	return prefix + context.definitionNameFor(toDocument, to)
}

func (context *GeneratorContext) NeedSchemaFor(typ crd.TypeIdent) {
	p := context.parser

	context.parser.NeedPackage(typ.Package)
	if _, knownSchema := context.parser.Schemata[typ]; knownSchema {
		return
	}

	info, knownInfo := p.Types[typ]
	fmt.Printf("info name = %s, marker = %t\n", info.Name, info.Markers.Get(objectMarker.Name) != nil)
	if !knownInfo {
		typ.Package.AddError(fmt.Errorf("unknown type %s", typ))
		return
	}

	// avoid tripping recursive schemata, like ManagedFields, by adding an empty WIP schema
	p.Schemata[typ] = apiext.JSONSchemaProps{}

	schemaCtx := newSchemaContext(typ.Package, context, p.AllowDangerousTypes)
	ctxForInfo := schemaCtx.ForInfo(info)

	pkgMarkers, err := markers.PackageMarkers(p.Collector, typ.Package)
	if err != nil {
		typ.Package.AddError(err)
	}
	ctxForInfo.PackageMarkers = pkgMarkers
	context.pkgMarkers[typ.Package] = pkgMarkers

	schema := infoToSchema(ctxForInfo)
	fmt.Printf("info name = %s, schema = %s\n", info.Name, schema)

	p.Schemata[typ] = *schema
}
