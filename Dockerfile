ARG GO_VERSION=1.26.0
FROM golang:${GO_VERSION}
ARG GO_VERSION
# We need to have both nodejs and go to build the binaries.
# We could use multi-stage builds but that would require significantly changing our Makefile.
RUN apt-get update
RUN curl -sL https://deb.nodesource.com/setup_24.x | bash -
RUN apt-get update && apt-get install -y nodejs

WORKDIR /copilot
COPY . .
RUN go env -w GOPROXY=direct
RUN make release
