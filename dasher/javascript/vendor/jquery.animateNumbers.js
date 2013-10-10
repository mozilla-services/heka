/***********
    Animates element's number to new number with commas
    Parameters:
        stop (number): number to stop on
        commas (boolean): turn commas on/off (default is true)
        duration (number): how long in ms (default is 1000)
        ease (string): type of easing (default is "swing", others are avaiable from jQuery's easing plugin
    Examples:
        $("#div").animateNumbers(1234, false, 500, "linear"); // half second linear without commas
        $("#div").animateNumbers(1234, true, 2000); // two second swing with commas
        $("#div").animateNumbers(4321); // one second swing with commas
    This fully expects an element containing an integer
    If the number is within copy then separate it with a span and target the span
    Inserts and accounts for commas during animation by default
***********/

(function($) {
    $.fn.animateNumbers = function(stop, commas, duration, ease) {
        return this.each(function() {
            var $this = $(this);
            var start = parseInt($this.text().replace(/,/g, ""));
            var lastText = null;
            commas = (commas === undefined) ? true : commas;
            $({value: start}).animate({value: stop}, {
                duration: duration == undefined ? 1000 : duration,
                easing: ease == undefined ? "swing" : ease,
                step: function() {
                    var text = Math.floor(this.value).toString();
                    if (commas) {
                        text = text.replace(/(\d)(?=(\d\d\d)+(?!\d))/g, "$1,");
                    }
                    // Only update the DOM if the text has changed
                    if (text !== lastText) {
                        $this.text(text);
                        lastText = text;
                    }
                },
                complete: function() {
                    if (parseInt($this.text()) !== stop) {
                        var text = stop.toString();
                        if (commas) {
                            text = text.replace(/(\d)(?=(\d\d\d)+(?!\d))/g, "$1,");
                        }
                        $this.text(text);
                    }
                }
            });
        });
    };
})(jQuery);
