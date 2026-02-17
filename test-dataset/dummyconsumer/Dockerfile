FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o myapp

##
## Runtime
##
FROM alpine:3.19

ARG USERNAME=steadybit
ARG USER_UID=10000

RUN adduser -u $USER_UID -D $USERNAME

USER $USER_UID

WORKDIR /app
COPY --from=builder /app/myapp /app/

CMD ["/app/myapp"]
