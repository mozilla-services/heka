Heka - Collect, aggregate, and visualize your data

An overview of the long term goals of the heka project can be found on
[ReadTheDocs](https://heka-docs.readthedocs.org/en/latest/).

Heka is still very nascent. This repo contains the heka/pipeline project,
which is the engine driving the agent and aggregator portions (both of which
run as the `hekad` daemon) of the overall Heka system, as well as some very
(very!) basic heka client code and benchmarking tools.

Heka is written in [Go](http://golang.org/). The easiest way to install Heka
is through [Heka-Build](https://github.com/mozilla-services/heka-build), which
will build the correct version of Go, set up a Go environment, and install all
required dependencies. Heka-Build also provides a mechanism for easily
integrating external plug-in packages into the generated `hekad`.

It is also possible to install Heka using Go's default `go get` mechanism.
Unfortunately, Heka won't work with Go 1.0.3, the most recent release at the
time of this writing. Heka will work with Go 1.1, once it is released. Heka
is tested against Go tip, revision 477b2e70b12d; it may or may not work with
more recent versions of Go tip. Assuming you have a version of Go that will
in fact work with Heka, you can install Heka into your current GOPATH using
the following command:

    go get github.com/mozilla-services/heka/hekad
