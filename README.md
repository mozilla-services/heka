Heka - Collect, aggregate, and visualize your data

An overview of the long term goals of the heka project can be found on
[ReadTheDocs](https://heka-docs.readthedocs.org/en/latest/).

Heka is still very nascent. This repo contains the heka/pipeline project,
which is the engine driving the agent and aggregator portions (both of which
run as the `hekad` daemon) of the overall Heka system, as well as some very
(very!) basic heka client code and benchmarking tools.

Rudimentary manual install process (coming soon, moar automation!):

- create go workspace
- export GOPATH to the root of your workspace
- mkdir -p $GOPATH/src/github.com/mozilla-services; cd $GOPATH/src/github.com/mozilla-services
- git clone https://github.com/mozilla-services/heka.git
- go get github.com/bitly/go-simplejson
- go get github.com/bitly/go-notify
- go get github.com/ugorji/go-msgpack
- go install heka/hekad

To run tests:
- go get code.google.com/p/gomock/gomock
- go get github.com/rafrombrc/gospec/src/gospec
- go test -i heka/pipeline
- go test heka/pipeline
- go test heka/message
