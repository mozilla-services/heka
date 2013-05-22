.. _release:

====================
Heka release process
====================

This document contains a description of the steps taken to make a release
of the Heka server.

#. Review intended release branch for correct version number (in
   `cmd/hekad/main.go`) and updated changelog (`CHANGES.txt`) and verify that
   the build succeeds and all tests pass.

#. Tag verified commit on intended release branch with appropriate version
   tag.

#. If this release is the highest released version number to date, the
   verified commit should be merged into the master branch.

#. Bump version number (in `cmd/hekad/main.go`) and add section for future
   release to changelog (`CHANGES.txt`). Commit "version bump" revision to
   the released version branch and push.

#. Build all required binary packages and upload to
   https://docs.services.mozilla.com/_static/binaries.
