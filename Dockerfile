ARG GOVERSION=1.12
FROM golang:${GOVERSION}

WORKDIR /src

COPY go.mod .
COPY go.sum .
 
RUN go mod download