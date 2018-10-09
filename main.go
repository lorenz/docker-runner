package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/fatih/color"
	"github.com/golang/glog"
	"gitlab.com/gitlab-org/gitlab-runner/common"
	"gitlab.com/gitlab-org/gitlab-runner/network"
)

type buildStreamItem struct {
	Stream string `json:"stream"`
	Aux    string `json:"aux"`
}

func main() {
	flag.Parse()
	glog.Infof("Starting Docker Builder")
	c := network.NewGitLabClient()
	url := os.Getenv("GITLAB_URL")
	creds := common.RunnerCredentials{
		URL:   url,
		Token: os.Getenv("GITLAB_RUNNER_TOKEN"),
	}
	metaFmt := color.New(color.FgGreen, color.Bold)
	failFmt := color.New(color.FgRed, color.Bold)
	registry := os.Getenv("REGISTRY")
	cli, _ := client.NewEnvClient()
	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		job, _ := c.RequestJob(common.RunnerConfig{
			Name:              "Docker Builder",
			Limit:             10,
			RunnerCredentials: creds,
		})
		if job == nil {
			continue
		}
		var traceBuf Buffer
		registryTag := fmt.Sprintf("%v/%v:%v", registry, job.Variables.Get("CI_PROJECT_PATH"), job.GitInfo.Sha)
		metaFmt.Fprintf(&traceBuf, "Building %v on Docker CI Builder\n", registryTag)
		updateTicker := time.NewTicker(5 * time.Second)
		go func() {
			for range updateTicker.C {
				traceBufTmp := traceBuf.String()
				c.UpdateJob(common.RunnerConfig{RunnerCredentials: creds}, &common.JobCredentials{ID: job.ID, Token: job.Token, URL: url}, common.UpdateJobInfo{
					ID:            job.ID,
					State:         "running",
					FailureReason: "",
					Trace:         &traceBufTmp,
				})
			}
		}()

		fail := func(err error) {
			updateTicker.Stop()
			failFmt.Fprintf(&traceBuf, "\n%v", err)
			traceBufTmp := traceBuf.String()
			c.UpdateJob(common.RunnerConfig{RunnerCredentials: creds}, &common.JobCredentials{ID: job.ID, Token: job.Token, URL: url}, common.UpdateJobInfo{
				ID:            job.ID,
				State:         "failed",
				FailureReason: "script_failure",
				Trace:         &traceBufTmp,
			})
		}

		var tags []string
		tags = append(tags, registryTag)
		if job.GitInfo.RefType == common.RefTypeTag {
			tags = append(tags, fmt.Sprintf("%v/%v:%v", registry, job.Variables.Get("CI_PROJECT_PATH"), job.GitInfo.Ref))
		}
		res, err := cli.ImageBuild(context.Background(), nil, types.ImageBuildOptions{
			RemoteContext: job.GitInfo.RepoURL,
			Tags:          tags,
		})
		if err != nil {
			glog.Error(err)
			continue
		}
		defer res.Body.Close()
		aux := func(msg jsonmessage.JSONMessage) {
			var result types.BuildResult
			if err := json.Unmarshal(*msg.Aux, &result); err != nil {
				glog.Warningf("Failed to parse AUX: %v", err)
				return
			}
		}
		err = jsonmessage.DisplayJSONMessagesStream(res.Body, &traceBuf, 0, false, aux)
		if err != nil {
			fail(err)
			continue
		}
		metaFmt.Fprintf(&traceBuf, "\nBuild successful\n")
		auxPush := func(msg jsonmessage.JSONMessage) {
			var result types.PushResult
			if err := json.Unmarshal(*msg.Aux, &result); err != nil {
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
			continue
		}

		metaFmt.Fprintf(&traceBuf, "\nImage push successful")
		updateTicker.Stop()

		traceBufTmp := traceBuf.String()
		c.UpdateJob(common.RunnerConfig{RunnerCredentials: creds}, &common.JobCredentials{ID: job.ID, Token: job.Token, URL: url}, common.UpdateJobInfo{
			ID:            job.ID,
			State:         "success",
			FailureReason: "",
			Trace:         &traceBufTmp,
		})
	}
}
