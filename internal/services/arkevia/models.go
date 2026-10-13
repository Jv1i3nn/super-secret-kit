package arkevia

type Document struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	IsNew    bool   `json:"is_new"`
}

type DocumentListResponse struct {
	Result []DocumentResult `json:"result"`
}

type DocumentResult struct {
	ID       string `json:"id"`
	Filename string `json:"Document.file_name"`
	IsNew    bool   `json:"isnew"`
}
