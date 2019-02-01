# Docker Runner
*A GitLab CI runner which securely and quickly builds container images*

## Installation guide
You can build the image using `docker build .`. Currently no prebuilt options are offered.
If you want Git LFS support, please also build the dind image in this repository.
A Kubernetes spec is provided as an example, please customize it for your own needs.

All configuration is done using environment variables. The following variables are available:

| Variable | Default | Description |
| -------- | ------- | ----------- |
| `GITLAB_URL` | *None* | The full URL to GitLab including protocol |
| `REGISTRY` | *None* | The registry to use, in Docker format (so just the hostname). Does not have to be a GitLab registry. |
| `GITLAB_RUNNER_TOKEN` | *None* | The runner token for this runner. Note that this runner doesn't perform registration. Use a Kubernetes secret claim or a separate registering application to obtain it. |
| `DOCKER_API_VERSION` | Highest supported version | Use this to limit the protocol version the Docker client attempts to use. For 18.06 a value of 1.38 is recommended. |

## User's guide
Use the following snippet in your `.gitlab-ci.yml`:
```yaml
build:
  stage: build
  script: [""]
  variables: # All are optional
    BUILD_DIR: some-dir # Build from a sub-directory and push under project-name/some-dir:tag
    BUILD_NAME: another-name # Overrides the image name from BUILD_DIR to project-name/another-name:tag
  tags:
    - docker # Or whatever tag you use for the builder
```
docker-runner will automatically grab the Dockerfile at the root of your project, make sure the base image (`FROM`) is up-to-date
and build it with full caching enabled and push it under the same name as the project on GitLab. No configuration necessary.

### Limitations
* No support for submodules
* No support for GitLab cache (it has its own) and artifacts

## Comparison with other approaches
Kaniko
  * \+ Much faster builds due to caching and single fetch directly to Docker daemon
  * \+ Better GitLab Integration
  * \- Doesn't respect resource limits set by K8s since builds are run by a separate Docker daemon

Docker on GitLab CI
  * \+ Faster builds due to direct fetch to Docker daemon
  * \+ Less configuration per project
  * \+ Guarantees base images are up-to-date
  * \+ Much safer, no known escapes from the build environment

External providers (Docker Hub, GCR, ACR)
  * \+ Generally faster
  * \+ Less confirguration
  * \+ Runs on your existing infrastructure
  * \- Worse resource isolation
