# Docker Runner

_A GitLab CI runner which securely and quickly builds container images_

## Installation guide

You can build the image using `docker build .`. Currently no prebuilt options are offered.
If you want Git LFS support, please also build the dind image in this repository.
A Kubernetes spec is provided as an example, please customize it for your own needs.

All configuration is done using environment variables. The following variables are available:

| Variable              | Default                   | Description                                                                                                                                                             |
| --------------------- | ------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GITLAB_URL`          | _None_                    | The full URL to GitLab including protocol                                                                                                                               |
| `REGISTRY`            | _None_                    | The registry to use, in Docker format (so just the hostname). If unset a GitLab registry is assumed and gitlab auth token and user is used for auth.                    |
| `GITLAB_RUNNER_TOKEN` | _None_                    | The runner token for this runner. Note that this runner doesn't perform registration. Use a Kubernetes secret claim or a separate registering application to obtain it. |
| `DOCKER_API_VERSION`  | Highest supported version | Use this to limit the protocol version the Docker client attempts to use. For 18.06 a value of 1.38 is recommended.                                                     |

## User's guide

Use the following snippet in your `.gitlab-ci.yml`:

```yaml
build:
  stage: build
  script: [""]
  variables: # All are optional
    BUILD_DIR: some-dir # Build from a sub-directory and push under project-name/some-dir:tag
    BUILD_NAME: another-name # Overrides the image name from BUILD_DIR to project-name/another-name:tag
    BUILD_FROM_ROOT: false # Build from root but search for Dockerfile in BUILD_DIR
  tags:
    - docker # Or whatever tag you use for the builder
```

docker-runner will automatically grab the Dockerfile at the root of your project, make sure the base image (`FROM`) is up-to-date
and build it with full caching enabled and push it under the same name as the project on GitLab. No configuration necessary.

For a custom registry it is possible to specify the auth user and password via build variables. It is recommended to set this as a [pipeline environment variable](https://docs.gitlab.com/ee/ci/variables/#variables).

| Variable            | Default | Description       |
| ------------------- | ------- | ----------------- |
| `REGISTRY_USER`     | _none_  | Registry user     |
| `REGISTRY_PASSWORD` | _none_  | Registry password |

### Limitations

- No support for submodules
- No support for GitLab cache (it has its own) and artifacts

## Comparison with other approaches

Kaniko

- \+ Much faster builds due to caching and single fetch directly to Docker daemon
- \+ Better GitLab Integration
- \- Doesn't respect resource limits set by K8s since builds are run by a separate Docker daemon

Docker on GitLab CI

- \+ Faster builds due to direct fetch to Docker daemon
- \+ Less configuration per project
- \+ Guarantees base images are up-to-date
- \+ Much safer, no known escapes from the build environment

External providers (Docker Hub, GCR, ACR)

- \+ Generally faster
- \+ Less confirguration
- \+ Runs on your existing infrastructure
- \- Worse resource isolation
