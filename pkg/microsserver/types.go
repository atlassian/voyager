package microsserver

type aliasInfo struct {
	Service service `json:"service"`
}

type service struct {
	Name  string `json:"name"`
	Owner string `json:"owner"`
}
