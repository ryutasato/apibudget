FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /apibudget-server ./cmd/apibudget-server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /apibudget-server /usr/local/bin/apibudget-server
COPY apibudget.example.yaml /etc/apibudget/apibudget.yaml
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["apibudget-server"]
