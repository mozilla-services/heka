TRAVIS README
====

[Travis CI](https://travis-ci.org) is used to run the heka test suite on every push and pull request to github. Those travis runs end with a docker build and push to the semi-official docker repository.

Travis Setup
---

The docker build relies on environmental variables being set for login. Here they are, in case you'd like to auto build heka for your fork. You'll need to install the travis command line tool and inject some docker variables:

```
gem install travis
travis login --org
travis env set DOCKER_EMAIL $YOUR_DOCKER_ACCT_EMAIL
travis env set DOCKER_USERNAME $YOUR_DOCKER_ACCT_USERNAME
travis env set DOCKER_PASSWORD $YOUR_DOCKER_ACCT_PASSWD
travis env set --public DOCKER_REPO_SLUG xrlx/heka
```

These values are used by the `docker/release_travis.sh`. The unit tests must pass for the docker image to be built.

Docker Images
---

The `$DOCKER_REPO_SLUG` is used to craft the `docker push $DOCKER_REPO_SLUG:$TAG`. The `$TAG` is determined using a git tag, if that's what kicked off travis, or more like the git branch which received a push. Pull requests will not generate image builds.