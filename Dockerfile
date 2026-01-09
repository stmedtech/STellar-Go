FROM node:20-alpine AS frontend-builder
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
RUN npm install
COPY frontend/ ./
RUN npm run build

FROM golang:1.24

RUN apt update && \
    apt install -y pciutils \
    gcc libgl1-mesa-dev xorg-dev libxkbcommon-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY Makefile ./Makefile
COPY p2p ./p2p
COPY core ./core
COPY cmd ./cmd
COPY assets.go ./assets.go
COPY stellar-client ./stellar-client
COPY --from=frontend-builder /app/dist ./frontend/dist
COPY frontend/assets.go ./frontend/assets.go
RUN CGO_ENABLED=1 GOOS=linux make build && mv ./build/stellar /stellar && rm -rf ./frontend

ENTRYPOINT [ "/stellar" ]