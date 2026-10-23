FROM golang:1.26.10-alpine

ARG USERNAME=vscode
ARG USER_UID=1000
ARG USER_GID=1000

RUN addgroup -g $USER_GID $USERNAME && \
    adduser -D -u $USER_UID -G $USERNAME $USERNAME

ENV GOPATH=/home/${USERNAME}/go \
    PATH=/home/${USERNAME}/go/bin:$PATH

RUN apk update && \
    apk add --no-cache jq curl git bash libstdc++ libgcc

RUN go install -v github.com/go-delve/delve/cmd/dlv@v1.26.3 && \
    go install -v golang.org/x/tools/gopls@v0.23.0 && \
    go install -v honnef.co/go/tools/cmd/staticcheck@v0.6.1

RUN chown -R ${USER_UID}:${USER_GID} /home/${USERNAME}/go && \
    mkdir -p /workspaces && \
    chown -R ${USER_UID}:${USER_GID} /workspaces

USER root
