FROM docker:28-dind

ARG GO_VERSION=1.26.0
ARG GINKGO_VERSION=v2.20.1

# Docker needs somewhere to put creds from docker login.
RUN wget https://github.com/docker/docker-credential-helpers/releases/download/v0.6.0/docker-credential-pass-v0.6.0-amd64.tar.gz && tar -xf docker-credential-pass-v0.6.0-amd64.tar.gz && chmod +x docker-credential-pass &&  mv docker-credential-pass /bin
ENV DOCKER_HOST=tcp://127.0.0.1:2375
ENV GOBIN /go/bin

# Install Go, Git and other dependencies so we can run ginkgo
RUN apk update && apk add bash gcc musl-dev openssl go git aws-cli jq

# Upgrade Go to the repo version. The version that's available in the base image is older than what we need.
RUN wget https://dl.google.com/go/go${GO_VERSION}.linux-amd64.tar.gz && tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz && rm go${GO_VERSION}.linux-amd64.tar.gz
ENV PATH=$PATH:/usr/local/go/bin
RUN apk del go

ENV GOPROXY=direct
RUN go install github.com/onsi/ginkgo/v2/ginkgo@${GINKGO_VERSION}

# Copy the binary
ADD bin/local/copilot-linux-amd64 /bin/copilot

# Add the e2e directory and the project go.mod
ADD e2e/ github.com/aproint/copilot-cli/e2e/
ADD go.mod github.com/aproint/copilot-cli/
ADD go.sum github.com/aproint/copilot-cli/

# Startup script which inits dockerd and then runs the e2e tests
COPY e2e/e2e.sh /bin/

ENTRYPOINT ["/bin/e2e.sh"]
