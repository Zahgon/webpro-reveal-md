FROM golang:1.23-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# reveal.js, mermaid and the highlight.js themes are embedded in the binary,
# so the runtime image needs neither node_modules nor network access.
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/reveal-md .

FROM alpine:3.20

COPY --from=build /out/reveal-md /usr/local/bin/reveal-md

EXPOSE 1948

WORKDIR /slides
ENTRYPOINT [ "reveal-md" ]
CMD [ "/slides" ]
