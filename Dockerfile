FROM golang:1.24

RUN apt update && \
    apt install -y pciutils \
    gcc libgl1-mesa-dev xorg-dev libxkbcommon-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY p2p ./p2p
COPY core ./core
COPY cmd ./cmd
COPY assets.go ./assets.go
COPY stellar-client ./stellar-client
RUN CGO_ENABLED=1 GOOS=linux go build ./cmd/stellar && mv ./stellar /stellar

ENTRYPOINT [ "/stellar" ]