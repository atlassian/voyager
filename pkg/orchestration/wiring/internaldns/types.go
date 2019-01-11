package internaldns

type Alias struct {
	AliasType string `json:"type"`
	Name      string `json:"name"`
}

type Spec struct {
	Aliases []Alias `json:"aliases"`
}
