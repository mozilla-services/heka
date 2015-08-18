.. _older_apis:

===================================
Transitional Filter and Output APIs
===================================

Heka's APIs for filter and output plugins have changed drastically between the
v0.9.X series and v0.10.0. To ease the transition, however, a very slightly
modified version of the older APIs will be supported throughout the v0.10.X
release series. These should allow users with pre-existing custom filter and
output plugins to upgrade their plugin code to work with the newly introduced
:ref:`disk buffering <buffering>` support with minimal effort.

These transitional APIs are documented in full below, but a brief summary is
that the APIs are identical to the filter and output APIs supported by the
v0.9.X series, with two exceptions:

* The plugin code must call the runner's :ref:`UpdateCursor
  <update_buffer_cursor>` method as appropriate to advance the disk buffer's
  cursor, exactly as required by the newer APIs.

* The ``pack.Recycle`` method now requires an ``error`` argument which signals
  to Heka whether or not processing of the message in question was successful.
  Using nil for this argument implies that the message was successfully
  processed. Using a regular, non-nil error (such as is returned by
  ``errors.New`` or ``fmt.Errorf``) implies that the message was unable to be
  processed but should not be tried again. Using a ``RetryMessageError`` (as is
  returned by ``pipeline.NewRetryMessageError``, see :ref:`here
  <retry_message_error>` implies that the message should be retried.

  In other words, the argument passed in to the Recycle call is used similarly
  to the return value of the :ref:`MessageProcessor interface's
  <message_processor_interface>` ``ProcessMessage`` function. The behavior is
  not identical, however. Use of a :ref:`PluginExitError <plugin_exit_error>`
  will not cause the plugin to exit, exiting is still signaled by returning
  from the plugin's ``Run`` method. Also, the RetryMessageError will only cause
  a message to be retried when buffering is in use. If a plugin does not have
  buffering turned on, then the value returned to the Recycle method will have
  no impact.

.. _old_filters:

Filters (transitional API)
==========================

Filter plugins are the message processing engine of the Heka system. They are
used to examine and process message contents, and trigger events based on those
contents in real time as messages are flowing through the Heka system.

The transitional filter plugin interface is just a single method::

    type OldFilter interface {
        Run(r FilterRunner, h PluginHelper) (err error)
    }

Like input plugins, old-style filters have a ``Run`` method which accepts a
runner and a helper, and which should not return until shutdown unless there's
an error condition. The similarities end there, however.

Filters should call ``runner.InChan()`` to gain access to the plugin's input
channel. A filter's input channel provides pointers to PipelinePack objects,
defined in `pipeline_runner.go <https://github.com/mozilla-
services/heka/blob/master/pipeline/pipeline_runner.go>`_, each of which
contains what should be by now a fully populated Message struct from which the
filter can extract any desired information.

Sometimes, while processing a message, a filter plugin will need to generate
one or more *new* messages, which can be handed back to the router to be
checked against all registered plugins' ``message_matcher`` rules.

To generate new messages, your filter must call
``PluginHelper.PipelinePack(msgLoopCount int)``. The ``msgloopCount`` value to
be passed in should be obtained from the ``MsgLoopCount`` value on the pack
that you're already holding, or possibly zero if the new message is being
triggered by a timed ticker instead of an incoming message. The PipelinePack
method returns two values, the first a ``*PipelinePack`` and the second an
error.  If all goes well, you'll get a pack ready for you to populate and a nil
error. If the loop count is greater than the configured maximum value (as a
safeguard against inadvertently creating infinite message loops), or if a pack
isn't available for some other reason, you'll get a nil pack and a non-nil
error.

Once a pack has been obtained, a filter plugin can populate its Message struct
using the various provided mutator methods. The pack can then be injected into
the Heka message router queue, where it will be checked against all plugin
message matchers, by passing it to the ``FilterRunner.Inject(pack
*PipelinePack)`` method. Note that, again as a precaution against message
looping, a plugin will not be allowed to inject a message which would get a
positive response from that plugin's own matcher.

Sometimes a filter will take a specific action triggered by a single incoming
message. There are many cases, however, when a filter is merely collecting or
aggregating data from the incoming messages, and instead will be sending out
reports on the data that has been collected at specific intervals. Heka has
built-in support for this use case. Any old style filter (or output) plugin can
include a ``ticker_interval`` config setting (in seconds, integers only), which
will automatically be extracted by Heka when the configuration is loaded. Then
from within your plugin code you can call ``FilterRunner.Ticker()`` and you
will get a channel (type ``<-chan time.Time``) that will send a tick at the
specified interval. Your plugin code can listen on the ticker channel and take
action as needed.

Observant readers might have noticed that, unlike the ``Input`` interface,
filters don't need to implement a ``Stop`` method. Instead, Heka will
communicate a shutdown event to filter plugins by closing the input channel
from which the filter is receiving PipelinePacks. When this channel is closed,
a filter should perform any necessary clean-up and then return from the Run
method with a nil value to indicate a clean exit.

Finally, there are two very important points that all authors of old style
filter plugins should keep in mind. The first has to do with updating the disk
buffer's queue cursor for cases when :ref:`buffering <buffering>` is in
use. Old style filters should extract the QueueCursor values from packs and
call ``FilterRunner.UpdateCursor`` as appropriate, just like filters that
support the newer APIs.

The second important point is that an old style filter's code *must* call
``pack.Recycle(err error)`` to tell Heka that it is through with the
pack. Failure to do so will cause Heka to not free up the packs for reuse,
exhausting the supply and eventually causing the entire pipeline to freeze.
The error value passed in to Recycle should be ``nil`` if the message was
processed successfully. The error value should be non-nil if the message
processing failed and the message should be dropped. The error value should be
an instance of :ref:`RetryMessageError <retry_message_error>` if the processing
failure was transient and message delivery should be tried again. Note that
redelivery will only happen in cases where the filter's configuration has
``use_buffering`` set to true.

.. _old_outputs:

Outputs (transitional API)
==========================

Output plugins are responsible for receiving Heka messages and using them to
generate interactions with the outside world.  The ``OldOutput`` interface is
nearly identical to the ``OldFilter`` interface::

    type OldOutput interface {
        Run(or OutputRunner, h PluginHelper) (err error)
    }

In fact, there are many ways in which old style filter and output plugins are
similar.  Like filters, outputs should call the ``InChan`` method on the
provided runner to get an input channel, which will feed PipelinePacks. Like
filters, outputs should listen on this channel until it is closed, at which
time they should perform any necessary clean-up and then return. And, like
filters, any old style output plugin with a ``ticker_interval`` value in the
configuration will use that value to create a ticker channel that can be
accessed using the runner's ``Ticker`` method. And, finally, outputs should
also be sure to call ``pack.Recycle`` (passing in an appropriate error value)
when they finish w/ a pack so that Heka knows the pack is freed up for reuse.

The primary way that outputs differ from filters, of course, is that outputs
need to serialize (or extract data from) the messages they receive and then
send that data to an external destination. The serialization (or data
extraction) should typically be performed by the output's specified encoder
plugin. The OutputRunner exposes the following methods to assist with this::

    Encode(pack *PipelinePack) (output []byte, err error)
    UsesFraming() bool
    Encoder() (encoder Encoder)

The ``Encode`` method will use the specified encoder to convert the pack's
message to binary data, then if ``use_framing`` was set to true in the
output's configuration it will prepend Heka's :ref:`stream_framing`. The
``UsesFraming`` method will tell you whether or not ``use_framing`` was set to
true. Finally, the ``Encoder`` method will return the actual encoder that was
registered. This is useful to check to make sure that an encoder was actually
registered, but generally you will want to use OutputRunner.Encode and not
Encoder.Encode, since the latter will not honor the output's ``use_framing``
specification.
