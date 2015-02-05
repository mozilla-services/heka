.. _config_rstencoder:

Restructured Text Encoder
=========================

Plugin Name: **RstEncoder**

The RstEncoder generates a `reStructuredText
<http://docutils.sourceforge.net/rst.html>`_ rendering of a Heka message,
including all fields and attributes. It is useful for debugging, especially
when coupled with a :ref:`config_log_output`.

Config:

<none>

Example:

.. code-block:: ini

	[RstEncoder]

	[LogOutput]
	message_matcher = "TRUE"
	encoder = "RstEncoder"
