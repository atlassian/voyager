package testutil

import (
	"io/ioutil"
	"path/filepath"

	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"
)

const (
	FixturesDir = "testdata"
)

func LoadFileFromTestData(filename string) ([]byte, error) {
	fullname := filepath.Join(FixturesDir, filename)
	body, err := ioutil.ReadFile(fullname)
	if err != nil {
		return nil, errors.Wrapf(err, "filed to read %s", fullname)
	}

	return body, nil
}

func LoadIntoStructFromTestData(filename string, obj interface{}) error {
	body, err := LoadFileFromTestData(filename)
	if err != nil {
		return errors.WithStack(err)
	}

	err = yaml.UnmarshalStrict(body, obj)
	if err != nil {
		return errors.Wrapf(err, "failed to parse YAML from %s", filename)
	}

	return nil
}
