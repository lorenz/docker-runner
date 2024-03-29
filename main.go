package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/fatih/color"
	"github.com/golang/glog"
)

type buildStreamItem struct {
	Stream string `json:"stream"`
	Aux    string `json:"aux"`
}

var registryInvalidChars = regexp.MustCompile("[^a-z0-9.-]+")
var tagInvalidChars = regexp.MustCompile(`[^\w.-]`) // https://github.com/docker/distribution/blob/master/reference/regexp.go#L37

func main() {
	flag.Parse()
	glog.Infof("Starting Docker Builder")
	c := NewGitlabRunnerClient(os.Getenv("GITLAB_URL"), os.Getenv("GITLAB_RUNNER_TOKEN"), VersionInfo{
		Name:    "Docker Runner",
		Version: "0.1",
	})
	color.NoColor = false // Force colorized output
	metaFmt := color.New(color.FgGreen, color.Bold)
	failFmt := color.New(color.FgRed, color.Bold)
	cli, _ := client.NewEnvClient()
	ticker := time.NewTicker(5 * time.Second)
	reserveStation := make(chan bool, 10)
	for range ticker.C {
		job, err := c.RequestJob()
		if err != nil {
			glog.Warningf("Failed to request job: %v", err)
		}
		if job == nil {
			continue
		}
		go func(job *JobResponse) {
			reserveStation <- true
			defer func() { _ = <-reserveStation }()
			var err error
			traceBuf := NewTrace()

			updateTicker := time.NewTicker(5 * time.Second)
			go func() {
				for range updateTicker.C {
					chunk, off := traceBuf.NextChunk()
					err := c.PatchTrace(job.ID, job.Token, chunk, off)
					if err == nil {
						traceBuf.CommitChunk()
					} else {
						traceBuf.AbortChunk()
						glog.Warningf("Failed to update trace: %v", err)
						continue
					}
					_, err = c.UpdateJob(job.ID, UpdateJobRequest{
						Token:         job.Token,
						State:         "running",
						FailureReason: "",
						Checksum:      traceBuf.Checksum(),
					})
					if err != nil {
						glog.Warningf("Failed to update job: %v", err)
					}
				}
			}()

			fail := func(err error) {
				updateTicker.Stop()
				failFmt.Fprintf(traceBuf, "%v", err)
				chunk, off := traceBuf.NextChunk()
				traceErr := c.PatchTrace(job.ID, job.Token, chunk, off)
				if traceErr == nil {
					traceBuf.CommitChunk()
				} else {
					traceBuf.AbortChunk()
					glog.Warningf("Failed to update trace: %v", traceErr)
				}
				_, err = c.UpdateJob(job.ID, UpdateJobRequest{
					Token:         job.Token,
					State:         "failed",
					FailureReason: "script_failure",
					Checksum:      traceBuf.Checksum(),
				})
				if err != nil {
					glog.Warningf("Failed to update job: %v", err)
				}
			}

			// Registry
			var gitlabRegistry bool
			var registry string
			if os.Getenv("REGISTRY") == "" {
				if job.Variables.Get("CI_REGISTRY") == "" {
					fail(errors.New("No Registry is specified"))
					return
				}
				registry = job.Variables.Get("CI_REGISTRY")
				gitlabRegistry = true
			} else {
				registry = os.Getenv("REGISTRY")
			}

			// Registry auth
			var authConfig types.AuthConfig
			if gitlabRegistry {
				authConfig = types.AuthConfig{
					Username: job.Variables.Get("CI_REGISTRY_USER"),
					Password: job.Token,
				}
			} else if job.Variables.Get("REGISTRY_USER") != "" && job.Variables.Get("REGISTRY_PASSWORD") != "" {
				authConfig = types.AuthConfig{
					Username: job.Variables.Get("REGISTRY_USER"),
					Password: job.Variables.Get("REGISTRY_PASSWORD"),
				}
			}

			// Image pull auth
			authConfigs := map[string]types.AuthConfig{}
			if (authConfig != types.AuthConfig{}) {
				authConfigs[registry] = authConfig
			}

			var subBuildName string
			var rootBuild bool
			if job.Variables.Get("BUILD_DIR") != "" {

				if job.Variables.Get("BUILD_FROM_ROOT") != "" {
					rootBuild, err = strconv.ParseBool(job.Variables.Get("BUILD_FROM_ROOT"))
					if err != nil {
						fail(errors.New("BUILD_FROM_ROOT is not a Bool"))
						return
					}
				}

				if job.Variables.Get("BUILD_NAME") != "" {
					if registryInvalidChars.MatchString(job.Variables.Get("BUILD_NAME")) {
						fail(errors.New("BUILD_NAME contains non-alphanumeric or upper case characters. This is not supported by Docker."))
						return
					}
					subBuildName = job.Variables.Get("BUILD_NAME")
				} else {
					subBuildName = registryInvalidChars.ReplaceAllString(strings.ToLower(job.Variables.Get("BUILD_DIR")), "")
				}
				subBuildName = fmt.Sprintf("/%v", subBuildName)
			}

			registryTag := fmt.Sprintf("%v/%v%v:%v", registry, strings.ToLower(job.Variables.Get("CI_PROJECT_PATH")), subBuildName, job.GitInfo.Sha)
			metaFmt.Fprintf(traceBuf, "Building %v on Docker CI Builder\n", registryTag)

			ciRefName := tagInvalidChars.ReplaceAllString(job.Variables.Get("CI_COMMIT_REF_NAME"), "")
			branchTag := fmt.Sprintf("%v/%v%v:%v", registry, strings.ToLower(job.Variables.Get("CI_PROJECT_PATH")), subBuildName, ciRefName)

			var tags []string
			tags = append(tags, registryTag)
			tags = append(tags, branchTag)

			buildArgs := make(map[string]*string)

			if job.Variables.Get("RELATIVE_FROM") != "" {
				relativeFromTag := fmt.Sprintf("%v/%v/%v:%v", registry, strings.ToLower(job.Variables.Get("CI_PROJECT_PATH")), job.Variables.Get("RELATIVE_FROM"), job.GitInfo.Sha)
				buildArgs["RELATIVE_FROM"] = &relativeFromTag
			}

			var res types.ImageBuildResponse
			if rootBuild {
				res, err = cli.ImageBuild(context.Background(), nil, types.ImageBuildOptions{
					RemoteContext: fmt.Sprintf("%v#%v:%v", job.GitInfo.RepoURL, job.GitInfo.Ref, ""),
					Tags:          tags,
					PullParent:    true,
					ForceRemove:   true,
					CPUShares:     0,
					AuthConfigs:   authConfigs,
					Dockerfile:    job.Variables.Get("BUILD_DIR") + "/Dockerfile",
					BuildArgs:     buildArgs,
				})
			} else {
				res, err = cli.ImageBuild(context.Background(), nil, types.ImageBuildOptions{
					RemoteContext: fmt.Sprintf("%v#%v:%v", job.GitInfo.RepoURL, job.GitInfo.Ref, job.Variables.Get("BUILD_DIR")),
					Tags:          tags,
					PullParent:    true,
					ForceRemove:   true,
					CPUShares:     0,
					AuthConfigs:   authConfigs,
					BuildArgs:     buildArgs,
				})
			}
			if err != nil {
				fail(err)
				glog.Error(err)
				return
			}
			defer res.Body.Close()
			aux := func(msg jsonmessage.JSONMessage) {
				var result types.BuildResult
				if err := json.Unmarshal(*msg.Aux, &result); err != nil {
					fail(err)
					glog.Warningf("Failed to parse AUX: %v", err)
					return
				}
			}
			err = jsonmessage.DisplayJSONMessagesStream(res.Body, traceBuf, 0, false, aux)
			if err != nil {
				fail(err)
				return
			}
			metaFmt.Fprintf(traceBuf, "Build successful\n\n")
			auxPush := func(msg jsonmessage.JSONMessage) {
				var result types.PushResult
				if err := json.Unmarshal(*msg.Aux, &result); err != nil {
					fail(err)
					glog.Warningf("Failed to parse AUX: %v", err)
					return
				}
			}

			// Image Push auth
			var dockerPushOptions types.ImagePushOptions
			if (authConfig != types.AuthConfig{}) {
				encodedAuthConfig, err := json.Marshal(authConfig)
				if err != nil {
					fail(err)
					return
				}
				dockerPushOptions.RegistryAuth = base64.URLEncoding.EncodeToString(encodedAuthConfig)
			} else {
				dockerPushOptions.RegistryAuth = "force X-Registry-Auth"
			}

			hasFailed := false
			for _, tag := range tags {
				res, err := cli.ImagePush(context.Background(), tag, dockerPushOptions)
				if err != nil {
					fail(err)
					hasFailed = true
					break
				}
				defer res.Close()
				err = jsonmessage.DisplayJSONMessagesStream(res, traceBuf, 0, false, auxPush)
				if err != nil {
					fail(err)
					hasFailed = true
					break
				}
			}
			if hasFailed {
				return
			}

			metaFmt.Fprintf(traceBuf, "Image push successful")
			updateTicker.Stop()

			chunk, off := traceBuf.NextChunk()
			err = c.PatchTrace(job.ID, job.Token, chunk, off)
			if err == nil {
				traceBuf.CommitChunk()
			} else {
				traceBuf.AbortChunk()
				glog.Warningf("Failed to update trace: %v", err)
			}

			_, err = c.UpdateJob(job.ID, UpdateJobRequest{
				Token:         job.Token,
				State:         "success",
				FailureReason: "",
				Checksum:      traceBuf.Checksum(),
			})
			if err != nil {
				glog.Warningf("Failed to update job: %v", err)
			}
		}(job)
	}
}
