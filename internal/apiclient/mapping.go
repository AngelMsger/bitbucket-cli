package apiclient

import (
	"strconv"
	"strings"
)

// mapping.go holds raw response shapes for both Bitbucket Cloud (REST 2.0) and
// Data Center (REST 1.0), plus the normalizers that flatten them into the
// package's flavor-agnostic models.

// --- Cloud (REST 2.0) raw shapes ---

type cloudLinks map[string]struct {
	Href string `json:"href"`
	Name string `json:"name,omitempty"`
}

func (l cloudLinks) href(key string) string {
	if v, ok := l[key]; ok {
		return v.Href
	}
	return ""
}

type cloudUser struct {
	Type        string     `json:"type"`
	AccountID   string     `json:"account_id"`
	UUID        string     `json:"uuid"`
	DisplayName string     `json:"display_name"`
	Nickname    string     `json:"nickname"`
	Username    string     `json:"username"`
	Links       cloudLinks `json:"links"`
}

func mapCloudUser(u cloudUser) User {
	return User{
		AccountID:   u.AccountID,
		UUID:        u.UUID,
		Name:        u.Username,
		Slug:        u.Nickname,
		DisplayName: u.DisplayName,
		Type:        u.Type,
	}
}

type cloudRepoCloneLink struct {
	Href string `json:"href"`
	Name string `json:"name"`
}

type cloudRepo struct {
	UUID        string `json:"uuid"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	IsPrivate   bool   `json:"is_private"`
	Language    string `json:"language"`
	Size        int64  `json:"size"`
	CreatedOn   string `json:"created_on"`
	UpdatedOn   string `json:"updated_on"`
	MainBranch  struct {
		Name string `json:"name"`
	} `json:"mainbranch"`
	Workspace struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	} `json:"workspace"`
	Links struct {
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
		Clone []cloudRepoCloneLink `json:"clone"`
	} `json:"links"`
}

func mapCloudRepo(r cloudRepo) *Repository {
	repo := &Repository{
		UUID:          r.UUID,
		Slug:          r.Slug,
		Name:          r.Name,
		Workspace:     r.Workspace.Slug,
		FullName:      r.FullName,
		Description:   r.Description,
		Private:       r.IsPrivate,
		DefaultBranch: r.MainBranch.Name,
		MainBranch:    r.MainBranch.Name,
		Language:      r.Language,
		Size:          r.Size,
		URL:           r.Links.HTML.Href,
		CreatedAt:     r.CreatedOn,
		UpdatedAt:     r.UpdatedOn,
	}
	for _, c := range r.Links.Clone {
		switch strings.ToLower(c.Name) {
		case "https":
			repo.CloneHTTPS = c.Href
		case "ssh":
			repo.CloneSSH = c.Href
		}
	}
	return repo
}

type cloudRepoList struct {
	Values  []cloudRepo `json:"values"`
	Page    int         `json:"page"`
	Pagelen int         `json:"pagelen"`
	Size    int         `json:"size"`
	Next    string      `json:"next"`
}

type cloudPRRef struct {
	Branch struct {
		Name string `json:"name"`
	} `json:"branch"`
	Commit struct {
		Hash string `json:"hash"`
	} `json:"commit"`
	Repository struct {
		FullName string `json:"full_name"`
		Name     string `json:"name"`
	} `json:"repository"`
}

func mapCloudPRRef(r cloudPRRef) PRRef {
	return PRRef{
		Branch:     r.Branch.Name,
		Commit:     r.Commit.Hash,
		Repository: r.Repository.FullName,
	}
}

type cloudParticipant struct {
	User           cloudUser `json:"user"`
	Role           string    `json:"role"`
	Approved       bool      `json:"approved"`
	State          string    `json:"state"`
	ParticipatedOn string    `json:"participated_on"`
}

func mapCloudParticipants(ps []cloudParticipant) []Participant {
	out := make([]Participant, 0, len(ps))
	for _, p := range ps {
		out = append(out, Participant{
			User:     mapCloudUser(p.User),
			Role:     p.Role,
			Approved: p.Approved,
			State:    p.State,
		})
	}
	return out
}

type cloudPR struct {
	ID           int                `json:"id"`
	Title        string             `json:"title"`
	Description  string             `json:"description"`
	State        string             `json:"state"`
	Author       cloudUser          `json:"author"`
	Source       cloudPRRef         `json:"source"`
	Destination  cloudPRRef         `json:"destination"`
	Reviewers    []cloudUser        `json:"reviewers"`
	Participants []cloudParticipant `json:"participants"`
	CommentCount int                `json:"comment_count"`
	TaskCount    int                `json:"task_count"`
	CreatedOn    string             `json:"created_on"`
	UpdatedOn    string             `json:"updated_on"`
	ClosedOn     string             `json:"closed_on"`
	MergeCommit  struct {
		Hash string `json:"hash"`
	} `json:"merge_commit"`
	Links struct {
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
	} `json:"links"`
}

func mapCloudPR(repo RepoRef, r cloudPR) *PullRequest {
	pr := &PullRequest{
		ID:           r.ID,
		Title:        r.Title,
		Description:  r.Description,
		State:        r.State,
		Author:       mapCloudUser(r.Author),
		Source:       mapCloudPRRef(r.Source),
		Destination:  mapCloudPRRef(r.Destination),
		Participants: mapCloudParticipants(r.Participants),
		Repository:   repo,
		URL:          r.Links.HTML.Href,
		CommentCount: r.CommentCount,
		TaskCount:    r.TaskCount,
		CreatedAt:    r.CreatedOn,
		UpdatedAt:    r.UpdatedOn,
		ClosedAt:     r.ClosedOn,
		MergeCommit:  r.MergeCommit.Hash,
	}
	for _, u := range r.Reviewers {
		pr.Reviewers = append(pr.Reviewers, Participant{User: mapCloudUser(u), Role: "REVIEWER"})
	}
	return pr
}

type cloudPRList struct {
	Values  []cloudPR `json:"values"`
	Pagelen int       `json:"pagelen"`
	Page    int       `json:"page"`
	Size    int       `json:"size"`
	Next    string    `json:"next"`
}

type cloudBranch struct {
	Name   string `json:"name"`
	Target struct {
		Hash    string `json:"hash"`
		Date    string `json:"date"`
		Message string `json:"message"`
	} `json:"target"`
}

func mapCloudBranch(b cloudBranch, defaultName string) Branch {
	return Branch{
		Name:        b.Name,
		Target:      b.Target.Hash,
		Default:     b.Name == defaultName,
		LastCommit:  b.Target.Hash,
		LastUpdated: b.Target.Date,
	}
}

type cloudBranchList struct {
	Values  []cloudBranch `json:"values"`
	Pagelen int           `json:"pagelen"`
	Page    int           `json:"page"`
	Size    int           `json:"size"`
	Next    string        `json:"next"`
}

type cloudCommit struct {
	Hash    string `json:"hash"`
	Message string `json:"message"`
	Date    string `json:"date"`
	Author  struct {
		Raw  string    `json:"raw"`
		User cloudUser `json:"user"`
	} `json:"author"`
	Parents []struct {
		Hash string `json:"hash"`
	} `json:"parents"`
	Links struct {
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
	} `json:"links"`
}

func mapCloudCommit(c cloudCommit) Commit {
	parents := make([]string, 0, len(c.Parents))
	for _, p := range c.Parents {
		parents = append(parents, p.Hash)
	}
	author := c.Author.Raw
	if author == "" && c.Author.User.DisplayName != "" {
		author = c.Author.User.DisplayName
	}
	return Commit{
		Hash:    c.Hash,
		Message: c.Message,
		Date:    c.Date,
		Author:  author,
		Parents: parents,
		URL:     c.Links.HTML.Href,
	}
}

type cloudCommitList struct {
	Values  []cloudCommit `json:"values"`
	Pagelen int           `json:"pagelen"`
	Next    string        `json:"next"`
}

type cloudCommentInline struct {
	Path string `json:"path"`
	From *int   `json:"from"`
	To   *int   `json:"to"`
}

type cloudComment struct {
	ID      int `json:"id"`
	Content struct {
		Raw  string `json:"raw"`
		HTML string `json:"html"`
	} `json:"content"`
	User   cloudUser           `json:"user"`
	Inline *cloudCommentInline `json:"inline"`
	Parent *struct {
		ID int `json:"id"`
	} `json:"parent"`
	CreatedOn string `json:"created_on"`
	UpdatedOn string `json:"updated_on"`
	Links     struct {
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
	} `json:"links"`
	Deleted    bool `json:"deleted"`
	Resolution *struct {
		Type string `json:"type"`
	} `json:"resolution"`
}

func mapCloudComment(prID int, c cloudComment) Comment {
	cm := Comment{
		ID:        c.ID,
		Content:   c.Content.Raw,
		Author:    mapCloudUser(c.User),
		PRID:      prID,
		URL:       c.Links.HTML.Href,
		CreatedAt: c.CreatedOn,
		UpdatedAt: c.UpdatedOn,
		Resolved:  c.Resolution != nil,
	}
	if c.Parent != nil {
		cm.ParentID = c.Parent.ID
	}
	if c.Inline != nil {
		a := &InlineAnchor{Path: c.Inline.Path}
		if c.Inline.From != nil {
			a.From = *c.Inline.From
			a.Line = *c.Inline.From
		}
		if c.Inline.To != nil {
			a.To = *c.Inline.To
			if a.Line == 0 {
				a.Line = *c.Inline.To
			}
		}
		cm.Inline = a
	}
	return cm
}

type cloudCommentList struct {
	Values []cloudComment `json:"values"`
	Next   string         `json:"next"`
}

// --- Data Center (REST 1.0) raw shapes ---

type dcUser struct {
	Name         string `json:"name"`
	EmailAddress string `json:"emailAddress"`
	ID           int    `json:"id"`
	DisplayName  string `json:"displayName"`
	Active       bool   `json:"active"`
	Slug         string `json:"slug"`
	Type         string `json:"type"`
}

func mapDCUser(u dcUser) User {
	return User{
		Name:        u.Name,
		Slug:        u.Slug,
		DisplayName: u.DisplayName,
		Email:       u.EmailAddress,
		Type:        u.Type,
	}
}

type dcProject struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	ID   int    `json:"id"`
}

type dcRepo struct {
	Slug        string    `json:"slug"`
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Public      bool      `json:"public"`
	State       string    `json:"state"`
	Project     dcProject `json:"project"`
	Links       struct {
		Clone []struct {
			Href string `json:"href"`
			Name string `json:"name"`
		} `json:"clone"`
		Self []struct {
			Href string `json:"href"`
		} `json:"self"`
	} `json:"links"`
}

func mapDCRepo(r dcRepo) *Repository {
	repo := &Repository{
		Slug:        r.Slug,
		Name:        r.Name,
		Workspace:   r.Project.Key,
		FullName:    r.Project.Key + "/" + r.Slug,
		Description: r.Description,
		Private:     !r.Public,
	}
	for _, c := range r.Links.Clone {
		switch strings.ToLower(c.Name) {
		case "http", "https":
			repo.CloneHTTPS = c.Href
		case "ssh":
			repo.CloneSSH = c.Href
		}
	}
	if len(r.Links.Self) > 0 {
		repo.URL = r.Links.Self[0].Href
	}
	return repo
}

type dcRepoList struct {
	Values     []dcRepo `json:"values"`
	Size       int      `json:"size"`
	Limit      int      `json:"limit"`
	Start      int      `json:"start"`
	IsLastPage bool     `json:"isLastPage"`
}

type dcPRRef struct {
	ID           string `json:"id"` // e.g. refs/heads/main
	DisplayID    string `json:"displayId"`
	LatestCommit string `json:"latestCommit"`
	Repository   dcRepo `json:"repository"`
}

func mapDCPRRef(r dcPRRef) PRRef {
	return PRRef{
		Branch:     r.DisplayID,
		Commit:     r.LatestCommit,
		Repository: r.Repository.Project.Key + "/" + r.Repository.Slug,
	}
}

type dcReviewer struct {
	User               dcUser `json:"user"`
	Role               string `json:"role"`
	Approved           bool   `json:"approved"`
	Status             string `json:"status"`
	LastReviewedCommit string `json:"lastReviewedCommit"`
}

type dcPR struct {
	ID           int          `json:"id"`
	Version      int          `json:"version"`
	Title        string       `json:"title"`
	Description  string       `json:"description"`
	State        string       `json:"state"`
	Open         bool         `json:"open"`
	Closed       bool         `json:"closed"`
	CreatedDate  int64        `json:"createdDate"`
	UpdatedDate  int64        `json:"updatedDate"`
	ClosedDate   int64        `json:"closedDate"`
	FromRef      dcPRRef      `json:"fromRef"`
	ToRef        dcPRRef      `json:"toRef"`
	Author       dcReviewer   `json:"author"`
	Reviewers    []dcReviewer `json:"reviewers"`
	Participants []dcReviewer `json:"participants"`
	Properties   struct {
		CommentCount int `json:"commentCount"`
	} `json:"properties"`
	Links struct {
		Self []struct {
			Href string `json:"href"`
		} `json:"self"`
	} `json:"links"`
}

func mapDCPR(repo RepoRef, r dcPR) *PullRequest {
	pr := &PullRequest{
		ID:           r.ID,
		Title:        r.Title,
		Description:  r.Description,
		State:        r.State,
		Author:       mapDCUser(r.Author.User),
		Source:       mapDCPRRef(r.FromRef),
		Destination:  mapDCPRRef(r.ToRef),
		Repository:   repo,
		CommentCount: r.Properties.CommentCount,
		CreatedAt:    epochToISO(r.CreatedDate),
		UpdatedAt:    epochToISO(r.UpdatedDate),
		ClosedAt:     epochToISO(r.ClosedDate),
	}
	if len(r.Links.Self) > 0 {
		pr.URL = r.Links.Self[0].Href
	}
	for _, rv := range r.Reviewers {
		pr.Reviewers = append(pr.Reviewers, Participant{
			User:     mapDCUser(rv.User),
			Role:     rv.Role,
			Approved: rv.Approved,
			State:    strings.ToLower(rv.Status),
		})
	}
	for _, p := range r.Participants {
		pr.Participants = append(pr.Participants, Participant{
			User:     mapDCUser(p.User),
			Role:     p.Role,
			Approved: p.Approved,
			State:    strings.ToLower(p.Status),
		})
	}
	return pr
}

type dcPRList struct {
	Values     []dcPR `json:"values"`
	Size       int    `json:"size"`
	Limit      int    `json:"limit"`
	Start      int    `json:"start"`
	IsLastPage bool   `json:"isLastPage"`
}

type dcBranch struct {
	ID           string `json:"id"`
	DisplayID    string `json:"displayId"`
	Type         string `json:"type"`
	LatestCommit string `json:"latestCommit"`
	IsDefault    bool   `json:"isDefault"`
}

func mapDCBranch(b dcBranch) Branch {
	return Branch{
		Name:       b.DisplayID,
		Target:     b.LatestCommit,
		Default:    b.IsDefault,
		LastCommit: b.LatestCommit,
	}
}

type dcBranchList struct {
	Values     []dcBranch `json:"values"`
	Size       int        `json:"size"`
	Limit      int        `json:"limit"`
	Start      int        `json:"start"`
	IsLastPage bool       `json:"isLastPage"`
}

type dcCommit struct {
	ID              string `json:"id"`
	DisplayID       string `json:"displayId"`
	Message         string `json:"message"`
	AuthorTimestamp int64  `json:"authorTimestamp"`
	Author          dcUser `json:"author"`
	Parents         []struct {
		ID string `json:"id"`
	} `json:"parents"`
}

func mapDCCommit(c dcCommit) Commit {
	parents := make([]string, 0, len(c.Parents))
	for _, p := range c.Parents {
		parents = append(parents, p.ID)
	}
	author := c.Author.DisplayName
	if author == "" {
		author = c.Author.Name
	}
	return Commit{
		Hash:    c.ID,
		Message: c.Message,
		Date:    epochToISO(c.AuthorTimestamp),
		Author:  author,
		Parents: parents,
	}
}

type dcCommitList struct {
	Values     []dcCommit `json:"values"`
	Size       int        `json:"size"`
	Limit      int        `json:"limit"`
	Start      int        `json:"start"`
	IsLastPage bool       `json:"isLastPage"`
}

type dcCommentAnchor struct {
	Line     int    `json:"line"`
	LineType string `json:"lineType"` // ADDED / REMOVED / CONTEXT
	FileType string `json:"fileType"` // FROM / TO
	Path     string `json:"path"`
	SrcPath  string `json:"srcPath"`
}

type dcComment struct {
	ID          int              `json:"id"`
	Version     int              `json:"version"`
	Text        string           `json:"text"`
	Author      dcUser           `json:"author"`
	CreatedDate int64            `json:"createdDate"`
	UpdatedDate int64            `json:"updatedDate"`
	Anchor      *dcCommentAnchor `json:"anchor"`
	State       string           `json:"state"`    // OPEN / RESOLVED / PENDING
	Severity    string           `json:"severity"` // NORMAL / BLOCKER (BLOCKER == task)
}

func mapDCComment(prID int, c dcComment) Comment {
	cm := Comment{
		ID:        c.ID,
		Content:   c.Text,
		Author:    mapDCUser(c.Author),
		PRID:      prID,
		CreatedAt: epochToISO(c.CreatedDate),
		UpdatedAt: epochToISO(c.UpdatedDate),
		Resolved:  c.State == "RESOLVED",
		Task:      c.Severity == "BLOCKER",
	}
	if c.Anchor != nil {
		cm.Inline = &InlineAnchor{
			Path: c.Anchor.Path,
			Line: c.Anchor.Line,
		}
		if c.Anchor.FileType == "FROM" {
			cm.Inline.From = c.Anchor.Line
		} else {
			cm.Inline.To = c.Anchor.Line
		}
	}
	return cm
}

type dcActivity struct {
	ID            int        `json:"id"`
	CreatedDate   int64      `json:"createdDate"`
	User          dcUser     `json:"user"`
	Action        string     `json:"action"`
	CommentAction string     `json:"commentAction"`
	Comment       *dcComment `json:"comment"`
}

type dcActivityList struct {
	Values     []dcActivity `json:"values"`
	IsLastPage bool         `json:"isLastPage"`
	Size       int          `json:"size"`
	Limit      int          `json:"limit"`
	Start      int          `json:"start"`
}

// epochToISO converts a Bitbucket Data Center millisecond epoch to an ISO-8601
// string. An empty / zero timestamp is rendered as "".
func epochToISO(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return strconv.FormatInt(ms, 10)
}
