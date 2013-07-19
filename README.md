# Heka

Data Acquisition and Processing Made Easy

Heka is a tool for collecting and collating data from a number of different
sources, performing "in-flight" processing of collected data, and delivering
the results to any number of destinations for further analysis.

More details can be found on the ReadTheDocs sites for the [Heka
project](http://heka-docs.readthedocs.org/) and for the [Heka
daemon](http://hekad.readthedocs.org/), and lower level package
documentation can be found on
[GoDoc](http://godoc.org/github.com/mozilla-services/heka). Questions can
be asked on the [mailing list](https://mail.mozilla.org/listinfo/heka) or
in the #heka channel on irc.mozilla.org.

Heka is written in [Go](http://golang.org/), but Heka plugins can be written
in either Go or [Lua](http://lua.org). The easiest way to compile Heka is by
using the build script in the root directory of the project, which will set up a 
Go environment, verify the prerequisites, and install all required dependencies.
The build process also provides a mechanism for easily integrating external 
plug-in packages into the generated `hekad`.  For more details and additional
installation options see 
[Installing](https://hekad.readthedocs.org/en/latest/installing.html).
