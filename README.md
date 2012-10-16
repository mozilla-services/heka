Heka Aggregator

Rudimentary manual install process (for now):

- create go workspace
- export GOPATH to the root of your workspace
- mkdir $GOPATH/src; cd $GOPATH/src
- git clone https://github.com/mozilla-services/heka.git
- go install heka/graterd
- go install heka/hekabench
