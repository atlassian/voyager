package app

import (
	"encoding/json"
	"io/ioutil"
	"time"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
)

type Audience struct {
	Name     string        `json:"name"`
	Secret   string        `json:"secret"`
	Lifetime time.Duration `json:"lifetime"`
}

type Secrets struct {
	Namespace string            `json:"namespace"`
	Labels    map[string]string `json:"labels"`
}

type Config struct {
	Secrets   Secrets    `json:"secrets"`
	Audiences []Audience `json:"audiences"`
}

func (a *Audience) UnmarshalJSON(data []byte) error {
	var rawAudience struct {
		Name     string `json:"name"`
		Secret   string `json:"secret"`
		Lifetime string `json:"lifetime"`
	}
	err := json.Unmarshal(data, &rawAudience)
	if err != nil {
		return errors.Wrap(err, "failed to parse audience")
	}
	a.Name = rawAudience.Name
	a.Secret = rawAudience.Secret
	a.Lifetime, err = time.ParseDuration(rawAudience.Lifetime)
	if err != nil {
		return errors.Wrapf(err, "failed to parse audience.lifetime %q, valid time units are: ns, ms, s, m, h", rawAudience.Lifetime)
	}
	return nil
}

func ReadConfig(configFile string) (Config, error) {
	config := Config{}
	doc, err := ioutil.ReadFile(configFile)

	if err != nil {
		return config, err
	}

	err = yaml.Unmarshal(doc, &config)

	return config, err
}
