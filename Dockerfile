FROM golang:1.24

RUN apt update && apt install pciutils -y

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
COPY p2p ./p2p
COPY core ./core
COPY cmd ./cmd
RUN cd /app/cmd/stellar && CGO_ENABLED=0 GOOS=linux go build -o /stellar

ENTRYPOINT [ "/stellar" ]