package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"

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

var registryInvalidChars = regexp.MustCompile("[^a-z0-9]+")

func main() {
	flag.Parse()
	glog.Infof("Starting Docker Builder")
	c := NewGitlabRunnerClient(os.Getenv("GITLAB_URL"), os.Getenv("GITLAB_RUNNER_TOKEN"), VersionInfo{
		Name:    "Docker Runner",
		Version: "0.1-dev",
	})
	color.NoColor = false // Force colorized output
	metaFmt := color.New(color.FgGreen, color.Bold)
	failFmt := color.New(color.FgRed, color.Bold)
	registry := os.Getenv("REGISTRY")
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
			var traceBuf Buffer

			updateTicker := time.NewTicker(5 * time.Second)
			go func() {
				for range updateTicker.C {
					traceBufTmp := traceBuf.String()
					_, err := c.UpdateJob(job.ID, UpdateJobRequest{
						Token:         job.Token,
						State:         "running",
						FailureReason: "",
						Trace:         &traceBufTmp,
					})
					if err != nil {
						glog.Warningf("Failed to update job: %v", err)
					}
				}
			}()

			fail := func(err error) {
				updateTicker.Stop()
				failFmt.Fprintf(&traceBuf, "%v", err)
				traceBufTmp := traceBuf.String()
				_, err = c.UpdateJob(job.ID, UpdateJobRequest{
					Token:         job.Token,
					State:         "failed",
					FailureReason: "script_failure",
					Trace:         &traceBufTmp,
				})
				if err != nil {
					glog.Warningf("Failed to update job: %v", err)
				}
			}

			var subBuildName string
			if job.Variables.Get("BUILD_DIR") != "" {
				if job.Variables.Get("BUILD_NAME") != "" {
					if registryInvalidChars.MatchString(job.Variables.Get("BUILD_NAME")) {
						fail(errors.New("BUILD_NAME contains non-alphanumeric or upper case characters. This is not supported by Docker."))
						return
					}
					subBuildName = job.Variables.Get("BUILD_NAME")
				} else {
					subBuildName = registryInvalidChars.ReplaceAllString(job.Variables.Get("BUILD_DIR"), "")
				}
				subBuildName = fmt.Sprintf("/%v", subBuildName)
			}
			registryTag := fmt.Sprintf("%v/%v%v:%v", registry, job.Variables.Get("CI_PROJECT_PATH"), subBuildName, job.GitInfo.Sha)
			metaFmt.Fprintf(&traceBuf, "Building %v on Docker CI Builder\n", registryTag)

			var tags []string
			tags = append(tags, registryTag)
			if job.GitInfo.RefType == RefTypeTag {
				tags = append(tags, fmt.Sprintf("%v/%v%v:%v", registry, job.Variables.Get("CI_PROJECT_PATH"), subBuildName, job.GitInfo.Ref))
			}
			res, err := cli.ImageBuild(context.Background(), nil, types.ImageBuildOptions{
				RemoteContext: fmt.Sprintf("%v#%v:%v", job.GitInfo.RepoURL, job.GitInfo.Ref, job.Variables.Get("BUILD_DIR")),
				Tags:          tags,
				PullParent:    true,
				ForceRemove:   true,
				CPUShares:     0,
			})
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
			err = jsonmessage.DisplayJSONMessagesStream(res.Body, &traceBuf, 0, false, aux)
			if err != nil {
				fail(err)
				return
			}
			metaFmt.Fprintf(&traceBuf, "Build successful\n\n")
			auxPush := func(msg jsonmessage.JSONMessage) {
				var result types.PushResult
				if err := json.Unmarshal(*msg.Aux, &result); err != nil {
					fail(err)
					glog.Warningf("Failed to parse AUX: %v", err)
					return
				}
			}
			hasFailed := false
			for _, tag := range tags {
				res, err := cli.ImagePush(context.Background(), tag, types.ImagePushOptions{RegistryAuth: "a"})
				if err != nil {
					fail(err)
					hasFailed = true
					break
				}
				defer res.Close()
				err = jsonmessage.DisplayJSONMessagesStream(res, &traceBuf, 0, false, auxPush)
				if err != nil {
					fail(err)
					hasFailed = true
					break
				}
			}
			if hasFailed {
				return
			}

			metaFmt.Fprintf(&traceBuf, "Image push successful")
			updateTicker.Stop()

			traceBufTmp := traceBuf.String()
			_, err = c.UpdateJob(job.ID, UpdateJobRequest{
				Token:         job.Token,
				State:         "success",
				FailureReason: "",
				Trace:         &traceBufTmp,
			})
			if err != nil {
				glog.Warningf("Failed to update job: %v", err)
			}
		}(job)
	}
}
