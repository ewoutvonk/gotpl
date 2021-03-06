FROM golang:1.10 as builder
WORKDIR /go/src/github.com/ewoutvonk/gotpl
RUN go get -d -v gopkg.in/yaml.v2
RUN go get -d -v github.com/Masterminds/sprig
RUN go get -d -v github.com/kubernetes/helm/pkg/strvals
RUN go get -d -v github.com/docopt/docopt-go
RUN go get -d -v k8s.io/helm/pkg/chartutil
RUN go get -d -v github.com/imdario/mergo
COPY tpl.go .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix gotpl -o gotpl .

FROM alpine
COPY --from=builder /go/src/github.com/ewoutvonk/gotpl/gotpl /usr/local/bin/gotpl
ENTRYPOINT ["/usr/local/bin/gotpl"]