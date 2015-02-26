.. _lpeg:

Lua Parsing Expression Grammars (LPeg)
======================================

Best practices (using Lpeg in the sandbox)
------------------------------------------
1) Read the `LPeg reference <http://www.inf.puc-rio.br/~roberto/lpeg/lpeg.html>`_

2) The 're' module is now available in the sandbox but the best practice is
   to use the LPeg syntax whenever possible (i.e., in Lua code). Why?

   - Consistency and readability of a single syntax.
   - Promotes more modular grammars.
   - Is easier to comment.

3) Do not use parentheses around function calls that take a single string argument.

.. code-block:: lua

    -- prefer
    lpeg.P"Literal"

    -- instead of
    lpeg.P("Literal")

4) When writing sub-grammars with an ordered choice (+) place each choice on its 
   own line; this make it easier to pick out the alternates.  Also, if possible
   order them from most frequent to least frequent use.

.. code-block:: lua

    local date_month = lpeg.P"0" * lpeg.R"19" 
                       + "1" * lpeg.R"02"

    -- The exception: when grouping alternates together in a higher level grammar.

    local log_grammar = (rfc3339 + iso8601) * log_severity * log_message

5) Use the locale patterns when matching standard character classes.

.. code-block:: lua

    -- prefer
    lpeg.digit

    -- instead of
    lpeg.R"09".

6) If a literal occurs within an expression avoid wrapping it in a function.

.. code-block:: lua

    -- prefer
    lpeg.digit * "Test"

    -- instead of
    lpeg.digit * lpeg.P"Test"

7) When creating a parser from an RFC standard mirror the ABNF grammar that is provided.

8) If creating a grammar that would also be useful to others, please consider contributing it back
   to the project, thanks.

9) Use the grammar tester http://lpeg.trink.com.

      
