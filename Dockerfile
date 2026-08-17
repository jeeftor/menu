FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w" \
    -o /menu \
    ./main.go

FROM gcr.io/distroless/static-debian12
COPY --from=builder /menu /menu
EXPOSE 8080
ENTRYPOINT ["/menu"]
CMD ["serve"]
