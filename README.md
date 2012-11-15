Heka Aggregator

Rudimentary manual install process (for now):

- create go workspace
- export GOPATH to the root of your workspace
- mkdir $GOPATH/src; cd $GOPATH/src
- git clone https://github.com/mozilla-services/heka.git
- go get github.com/bitly/go-simplejson
- go get github.com/bitly/go-notify
- go get github.com/ugorji/go-msgpack
- go install heka/graterd
- go install heka/flood

To run tests:
- go get code.google.com/p/gomock/gomock
- go get github.com/rafrombrc/gospec/src/gospec
- go test -i heka/pipeline
- go test heka/pipeline
- go test heka/message