FROM golang:1.25.1-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY server/ ./server/
RUN go test -v ./server
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w -extldflags "-static"' -trimpath -tags netgo -installsuffix netgo -o server ./server


FROM scratch
WORKDIR /app
COPY --from=builder /app/server /app/
USER 1000:1000
EXPOSE 8080
CMD ["/app/server"]