package microsserver

type AliasInfo struct {
	Service service `json:"service"`
}

type service struct {
	Name  string `json:"name"`
	Owner string `json:"owner"`
}
