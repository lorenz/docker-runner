package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// Taken from GitLab Runner common module (can't use that directly because of insane dependencies)

type FeaturesInfo struct {
	Variables               bool `json:"variables"`
	Image                   bool `json:"image"`
	Services                bool `json:"services"`
	Artifacts               bool `json:"artifacts"`
	Cache                   bool `json:"cache"`
	Shared                  bool `json:"shared"`
	UploadMultipleArtifacts bool `json:"upload_multiple_artifacts"`
	UploadRawArtifacts      bool `json:"upload_raw_artifacts"`
	Session                 bool `json:"session"`
	Terminal                bool `json:"terminal"`
}

type VersionInfo struct {
	Name         string       `json:"name,omitempty"`
	Version      string       `json:"version,omitempty"`
	Revision     string       `json:"revision,omitempty"`
	Platform     string       `json:"platform,omitempty"`
	Architecture string       `json:"architecture,omitempty"`
	Executor     string       `json:"executor,omitempty"`
	Shell        string       `json:"shell,omitempty"`
	Features     FeaturesInfo `json:"features"`
}

type JobRequest struct {
	Info       VersionInfo  `json:"info,omitempty"`
	Token      string       `json:"token,omitempty"`
	LastUpdate string       `json:"last_update,omitempty"`
	Session    *SessionInfo `json:"session,omitempty"`
}

type SessionInfo struct {
	URL           string `json:"url,omitempty"`
	Certificate   string `json:"certificate,omitempty"`
	Authorization string `json:"authorization,omitempty"`
}

type JobInfo struct {
	Name        string `json:"name"`
	Stage       string `json:"stage"`
	ProjectID   int    `json:"project_id"`
	ProjectName string `json:"project_name"`
}

type GitInfoRefType string

const (
	RefTypeBranch GitInfoRefType = "branch"
	RefTypeTag    GitInfoRefType = "tag"
)

type GitInfo struct {
	RepoURL   string         `json:"repo_url"`
	Ref       string         `json:"ref"`
	Sha       string         `json:"sha"`
	BeforeSha string         `json:"before_sha"`
	RefType   GitInfoRefType `json:"ref_type"`
}

type RunnerInfo struct {
	Timeout int `json:"timeout"`
}

type StepScript []string

type StepName string

const (
	StepNameScript      StepName = "script"
	StepNameAfterScript StepName = "after_script"
)

type StepWhen string

const (
	StepWhenOnFailure StepWhen = "on_failure"
	StepWhenOnSuccess StepWhen = "on_success"
	StepWhenAlways    StepWhen = "always"
)

type CachePolicy string

const (
	CachePolicyUndefined CachePolicy = ""
	CachePolicyPullPush  CachePolicy = "pull-push"
	CachePolicyPull      CachePolicy = "pull"
	CachePolicyPush      CachePolicy = "push"
)

type Step struct {
	Name         StepName   `json:"name"`
	Script       StepScript `json:"script"`
	Timeout      int        `json:"timeout"`
	When         StepWhen   `json:"when"`
	AllowFailure bool       `json:"allow_failure"`
}

type Steps []Step

type Image struct {
	Name       string   `json:"name"`
	Alias      string   `json:"alias,omitempty"`
	Command    []string `json:"command,omitempty"`
	Entrypoint []string `json:"entrypoint,omitempty"`
}

type Services []Image

type ArtifactPaths []string

type ArtifactWhen string

const (
	ArtifactWhenOnFailure ArtifactWhen = "on_failure"
	ArtifactWhenOnSuccess ArtifactWhen = "on_success"
	ArtifactWhenAlways    ArtifactWhen = "always"
)

func (when ArtifactWhen) OnSuccess() bool {
	return when == "" || when == ArtifactWhenOnSuccess || when == ArtifactWhenAlways
}

func (when ArtifactWhen) OnFailure() bool {
	return when == ArtifactWhenOnFailure || when == ArtifactWhenAlways
}

type ArtifactFormat string

const (
	ArtifactFormatDefault ArtifactFormat = ""
	ArtifactFormatZip     ArtifactFormat = "zip"
	ArtifactFormatGzip    ArtifactFormat = "gzip"
	ArtifactFormatRaw     ArtifactFormat = "raw"
)

type Artifact struct {
	Name      string         `json:"name"`
	Untracked bool           `json:"untracked"`
	Paths     ArtifactPaths  `json:"paths"`
	When      ArtifactWhen   `json:"when"`
	Type      string         `json:"artifact_type"`
	Format    ArtifactFormat `json:"artifact_format"`
	ExpireIn  string         `json:"expire_in"`
}

type Artifacts []Artifact

type Cache struct {
	Key       string        `json:"key"`
	Untracked bool          `json:"untracked"`
	Policy    CachePolicy   `json:"policy"`
	Paths     ArtifactPaths `json:"paths"`
}

func (c Cache) CheckPolicy(wanted CachePolicy) (bool, error) {
	switch c.Policy {
	case CachePolicyUndefined, CachePolicyPullPush:
		return true, nil
	case CachePolicyPull, CachePolicyPush:
		return wanted == c.Policy, nil
	}

	return false, fmt.Errorf("Unknown cache policy %s", c.Policy)
}

type Caches []Cache

type Credentials struct {
	Type     string `json:"type"`
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type DependencyArtifactsFile struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

type Dependency struct {
	ID            int                     `json:"id"`
	Token         string                  `json:"token"`
	Name          string                  `json:"name"`
	ArtifactsFile DependencyArtifactsFile `json:"artifacts_file"`
}

type Dependencies []Dependency

type GitlabFeatures struct {
	TraceSections bool `json:"trace_sections"`
}

type JobResponse struct {
	ID            int            `json:"id"`
	Token         string         `json:"token"`
	AllowGitFetch bool           `json:"allow_git_fetch"`
	JobInfo       JobInfo        `json:"job_info"`
	GitInfo       GitInfo        `json:"git_info"`
	RunnerInfo    RunnerInfo     `json:"runner_info"`
	Variables     JobVariables   `json:"variables"`
	Steps         Steps          `json:"steps"`
	Image         Image          `json:"image"`
	Services      Services       `json:"services"`
	Artifacts     Artifacts      `json:"artifacts"`
	Cache         Caches         `json:"cache"`
	Credentials   []Credentials  `json:"credentials"`
	Dependencies  Dependencies   `json:"dependencies"`
	Features      GitlabFeatures `json:"features"`
}

type JobVariable struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Public   bool   `json:"public"`
	Internal bool   `json:"-"`
	File     bool   `json:"file"`
}

type JobVariables []JobVariable

func (b JobVariable) String() string {
	return fmt.Sprintf("%s=%s", b.Key, b.Value)
}

func (b JobVariables) PublicOrInternal() (variables JobVariables) {
	for _, variable := range b {
		if variable.Public || variable.Internal {
			variables = append(variables, variable)
		}
	}
	return variables
}

func (b JobVariables) StringList() (variables []string) {
	for _, variable := range b {
		variables = append(variables, variable.String())
	}
	return variables
}

func (b JobVariables) Get(key string) string {
	switch key {
	case "$":
		return key
	case "*", "#", "@", "!", "?", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
		return ""
	}
	for i := len(b) - 1; i >= 0; i-- {
		if b[i].Key == key {
			return b[i].Value
		}
	}
	return ""
}

func (b JobVariables) ExpandValue(value string) string {
	return os.Expand(value, b.Get)
}

func (b JobVariables) Expand() (variables JobVariables) {
	for _, variable := range b {
		variable.Value = b.ExpandValue(variable.Value)
		variables = append(variables, variable)
	}
	return variables
}

func ParseVariable(text string) (variable JobVariable, err error) {
	keyValue := strings.SplitN(text, "=", 2)
	if len(keyValue) != 2 {
		err = errors.New("missing =")
		return
	}
	variable = JobVariable{
		Key:   keyValue[0],
		Value: keyValue[1],
	}
	return
}

type UpdateJobRequest struct {
	Info          VersionInfo      `json:"info,omitempty"`
	Token         string           `json:"token,omitempty"`
	State         JobState         `json:"state,omitempty"`
	FailureReason JobFailureReason `json:"failure_reason,omitempty"`
	Trace         *string          `json:"trace,omitempty"`
}

type JobState string
type JobFailureReason string

const (
	Pending JobState = "pending"
	Running JobState = "running"
	Failed  JobState = "failed"
	Success JobState = "success"
)

const (
	NoneFailure         JobFailureReason = ""
	ScriptFailure       JobFailureReason = "script_failure"
	RunnerSystemFailure JobFailureReason = "runner_system_failure"
)

// GitlabRunnerClient is a minimal API client for the Gitlab Runner API
type GitlabRunnerClient struct {
	token       string
	versionInfo VersionInfo
	baseURL     string
}

// From https://stackoverflow.com/questions/8689425/remove-last-character-of-a-string
func trimSuffix(s, suffix string) string {
	if strings.HasSuffix(s, suffix) {
		s = s[:len(s)-len(suffix)]
	}
	return s
}

func NewGitlabRunnerClient(baseURL string, token string, versionInfo VersionInfo) *GitlabRunnerClient {
	return &GitlabRunnerClient{
		token:       token,
		versionInfo: versionInfo,
		baseURL:     trimSuffix(baseURL, "/"),
	}
}

func (c *GitlabRunnerClient) RequestJob() (*JobResponse, error) {
	bodyData := JobRequest{
		Info:  c.versionInfo,
		Token: c.token,
	}
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(&bodyData); err != nil {
		panic(err) // Is guaranteed by invariant
	}
	res, err := http.Post(fmt.Sprintf("%v/api/v4/jobs/request", c.baseURL), "application/json; charset=utf-8", buf)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusCreated:
		// We have a job, decode and return it
		var req JobResponse
		if err := json.NewDecoder(res.Body).Decode(&req); err != nil {
			return nil, fmt.Errorf("Failed to decode job: %v", err)
		}
		return &req, nil
	case http.StatusNoContent:
		return nil, nil // There is no job for us
	case http.StatusForbidden:
		return nil, fmt.Errorf("Failed to get job: GitLab denied access")
	default:
		return nil, fmt.Errorf("Failed to get job: Got HTTP %v", res.StatusCode)
	}
	var req JobResponse
	if err := json.NewDecoder(res.Body).Decode(&req); err != nil {
		return nil, fmt.Errorf("Failed to decode job: %v", err)
	}
	return &req, nil
}

func (c *GitlabRunnerClient) UpdateJob(id int, req UpdateJobRequest) (bool, error) {
	req.Info = c.versionInfo
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(&req); err != nil {
		panic(err) // Is guaranteed by invariant
	}
	res, err := http.Post(fmt.Sprintf("%v/api/v4/jobs/%v", c.baseURL, id), "application/json; charset=utf-8", buf)
	if err != nil {
		return false, err
	}
	res.Body.Close()

	jobStatus := res.Header.Get("Job-Status")
	if jobStatus == "failed" || jobStatus == "canceled" {
		return true, nil
	}

	switch res.StatusCode {
	case http.StatusOK:
		return false, nil
	case http.StatusForbidden:
		return true, fmt.Errorf("Failed update job: GitLab denied access")
	case http.StatusNotFound:
		return true, fmt.Errorf("Failed update job: Job got removed")
	default:
		return false, fmt.Errorf("Failed update job: Got HTTP %v", res.StatusCode)
	}

}
