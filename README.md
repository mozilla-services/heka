Heka Aggregator

Rudimentary manual install process (for now):

- create go workspace
- export GOPATH to the root of your workspace
- mkdir $GOPATH/src; cd $GOPATH/src
- git clone https://github.com/mozilla-services/heka.git
- go get github.com/bitly/go-simplejson
- go get code.google.com/p/gomock/gomock
- go get code.google.com/p/gomock/mockgen
- go get github.com/orfjackal/gospec/src/gospec
- go get github.com/peterbourgon/g2s
- go install heka/graterd
- go install heka/hekabench

Running the graterd:

- cd $GOPATH/src
- go run heka/graterd/main.go
