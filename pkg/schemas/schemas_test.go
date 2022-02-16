package schemas

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"emperror.dev/errors"
	api "fybrik.io/fybrik/manager/apis/app/v1alpha1"
	"fybrik.io/fybrik/pkg/model/taxonomy"
	"github.com/xeipuuv/gojsonschema"
)

func TestValidApp(t *testing.T) {

	// schemaPath := "./taxonomy/fybrik_application.json"
	schemaPath := "/home/mohammadtn/json-schema-generator/fybrik/tmp/FybrikApplicationSpec.json"
	schemaPath, err := filepath.Abs(schemaPath)
	if err != nil {
		t.Errorf("error %v\n", err)
		return
	}
	resourceJSON, err := createResource()
	if err != nil {
		t.Errorf("error M %v\n", err)
		return
	}
	// Validate resource against taxonomy
	taxonomyLoader := gojsonschema.NewReferenceLoader("file://" + schemaPath)
	documentLoader := gojsonschema.NewBytesLoader(resourceJSON)
	result, err := gojsonschema.Validate(taxonomyLoader, documentLoader)
	if err != nil {
		e := errors.Wrap(err, "could not validate resource against the provided schema, check files at "+filepath.Dir(schemaPath))
		t.Errorf(" %v\n", e)
		return
	}
	errors := result.Errors()
	for i, e := range errors {
		t.Errorf("Error %d %v\n", i, e)
		return
	}
	if !result.Valid() {
		t.Error("test failed")
	}
}

func TestInvalidApp(t *testing.T) {
	schemaPath := "/home/mohammadtn/json-schema-generator/fybrik/tmp/FybrikApplicationSpec.json"
	schemaPath, err := filepath.Abs(schemaPath)
	if err != nil {
		t.Errorf("error %v\n", err)
		return
	}
	resourceJSON, err := createInvalidResource()
	if err != nil {
		t.Errorf("error M %v\n", err)
		return
	}
	// Validate resource against taxonomy
	taxonomyLoader := gojsonschema.NewReferenceLoader("file://" + schemaPath)
	documentLoader := gojsonschema.NewBytesLoader(resourceJSON)
	result, err := gojsonschema.Validate(taxonomyLoader, documentLoader)
	if err != nil {
		e := errors.Wrap(err, "could not validate resource against the provided schema, check files at "+filepath.Dir(schemaPath))
		t.Errorf(" %v\n", e)
		return
	}
	errors := result.Errors()
	for _, e := range errors {
		if e.String() == "appInfo: Invalid type. Expected: object, given: null" {
			return
		}
	}
	if result.Valid() {
		t.Error("test failed")
	}
}

func createResource() ([]byte, error) {
	appInfo := taxonomy.AppInfo{}
	appInfo.Items = map[string]interface{}{}
	appInfo.Items["intent"] = "Fraud Detection"
	spec := &api.FybrikApplicationSpec{}
	spec.AppInfo = appInfo
	spec.Data = []api.DataContext{}
	buf, err := json.Marshal(spec)
	return buf, err
}

func createInvalidResource() ([]byte, error) {
	spec := &api.FybrikApplicationSpec{}
	spec.Data = []api.DataContext{}
	// Convert Fybrik application Go struct to JSON
	buf, err := json.Marshal(spec)
	return buf, err
}
