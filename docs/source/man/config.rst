=====
hekad
=====

Description
===========

.. include:: /configuration.rst
    :start-after: start-hekad-config
    :end-before: end-hekad-config

Full documentation on available plugins and settings for each one are
in the hekad.plugin(5) pages.

Example hekad.toml File
=======================

.. include:: /configuration.rst
    :start-after: start-hekad-toml
    :end-before: end-hekad-toml

Roles
=====

`hekad` is frequently configured for various roles in a larger cluster:

.. include:: /configuration.rst
    :start-after: start-roles
    :end-before: end-roles

A single `hekad` daemon could act as all the roles in smaller
deployments.

.. include:: /configuration.rst
    :start-after: start-restarting
    :end-before: end-restarting


See Also
========

hekad(1), hekad.plugin(5)
