FROM golang:1.24.1-alpine AS builder
WORKDIR /src

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/employee-factory ./cmd/employee-factory

FROM gcr.io/distroless/static:nonroot
WORKDIR /app

COPY --from=builder /out/employee-factory /app/employee-factory

EXPOSE 8080
ENV HTTP_ADDR=:8080

USER nonroot:nonroot
ENTRYPOINT ["/app/employee-factory"]
