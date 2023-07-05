# BUILD IMAGE --------------------------------------------------------
FROM golang:1.19-alpine3.16 as builder

# Get build tools and required header files
RUN apk add --no-cache build-base
RUN apk add --no-cache bash
RUN apk add --no-cache git

WORKDIR /app
COPY . .

# Build the final node binary
RUN go build

# ACTUAL IMAGE -------------------------------------------------------

FROM alpine:3.16

# color, nocolor, json
ENV GOLOG_LOG_FMT=nocolor

# go-waku default ports
EXPOSE 6000

COPY --from=builder /app/discv5 /usr/bin/discv5

ENTRYPOINT ["/usr/bin/discv5"]
