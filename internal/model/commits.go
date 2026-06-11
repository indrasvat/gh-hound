package model

// Commit is one suspect in a regression range: who touched the trail
// between the last clean run and the first dirty one.
type Commit struct {
	SHA     string `json:"sha"`
	Author  string `json:"author"`
	Message string `json:"message"`
}

// CommitRange is the compare-API view of base...head. Commits may be
// capped by the caller; TotalCommits always reports the full range.
type CommitRange struct {
	TotalCommits int
	HTMLURL      string
	Commits      []Commit
}
