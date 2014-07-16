.. _message_matcher:

======================
Message Matcher Syntax
======================

Message matching is done by the `hekad` router to choose an appropriate
filter(s) to run. Every filter that matches will get a copy of the
message.

Examples
========

- Type == "test" && Severity == 6
- (Severity == 7 || Payload == "Test Payload") && Type == "test"
- Fields[foo] != "bar"
- Fields[foo][1][0] == 'alternate'
- Fields[MyBool] == TRUE
- TRUE
- Fields[created] =~ /%TIMESTAMP%/
- Fields[widget] != NIL

Relational Operators
====================

- **==** equals
- **!=** not equals
- **>** greater than
- **>=** greater than equals
- **<** less than
- **<=** less than equals
- **=~** regular expression match
- **!~** regular expression negated match

Logical Operators
=================

- Parentheses are used for grouping expressions
- **&&** and (higher precedence)
- **||** or

Boolean
=======

- **TRUE**
- **FALSE**

Constants
=========

- **NIL** used to test the existence (!=) or non-existence (==) of a field variable
    - must be placed on the right side of the comparison  e.g., Fields[widget] == NIL

Message Variables
=================

- All message variables must be on the left hand side of the relational
  comparison
- String
    - **Uuid**
    - **Type**
    - **Logger**
    - **Payload**
    - **EnvVersion**
    - **Hostname**
- Numeric
    - **Timestamp**
    - **Severity**
    - **Pid**
- Fields
    - **Fields[_field_name_]** (shorthand for Field[_field_name_][0][0])
    - **Fields[_field_name_][_field_index_]** (shorthand for Field[_field_name_][_field_index_][0])
    - **Fields[_field_name_][_field_index_][_array_index_]**
    - If a field type is mis-match for the relational comparison, false will be returned e.g., Fields[foo] == 6 where 'foo' is a string

Quoted String
=============

- single or double quoted strings are allowed
- must be placed on the right side of a relational comparison e.g., Type == 'test'

Regular Expression String
=========================

- enclosed by forward slashes
- must be placed on the right side of the relational comparison e.g., Type =~ /test/
- capture groups will be ignored

.. seealso:: `Regular Expression re2 syntax <http://code.google.com/p/re2/wiki/Syntax>`_
