package testutil

import (
	"io/ioutil"
	"path/filepath"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
)

const (
	FixturesDir = "testdata"
)

func LoadFileFromTestData(filename string) ([]byte, error) {
	body, err := ioutil.ReadFile(filepath.Join(FixturesDir, filename))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return body, nil
}

func LoadIntoStructFromTestData(filename string, obj interface{}) error {
	body, err := LoadFileFromTestData(filename)
	if err != nil {
		return errors.WithStack(err)
	}

	err = yaml.Unmarshal(body, obj)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
