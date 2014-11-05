GeoIpDecoder
============

.. versionadded:: 0.6

Decoder plugin that generates GeoIP data based on the IP address of a specified field. It uses the Go project: https://github.com/abh/geoip as a wrapper around MaxMind's geoip-api-c library.
This decoder assumes you have downloaded and installed the `geoip-api-c <https://github.com/maxmind/geoip-api-c/releases/>`_ library from MaxMind's website.
Currently, only the GeoLiteCity database is supported, which you must also download and install yourself into a location to be referenced by the db_file config option. 
By default the database file is opened using "GEOIP_MEMORY_CACHE" mode. This setting is hard-coded into the wrapper's geoip.go file. You will need to manually override that code 
if you want to specify one of the other modes listed `here: <https://github.com/maxmind/geoip-api-c/blob/master/README.md#memory-caching-and-other-options/>`_ 

.. note::
        If you are using this with the ES output you will likely need to specify the raw_bytes_field option for the target_field specified. This is required to preserve the formatting of the JSON object.

Config:

- db_file:
        The location of the GeoLiteCity.dat database. Defaults to "/var/cache/hekad/GeoLiteCity.dat"

- source_ip_field:
        The name of the field containing the IP address you want to derive the location for.

- target_field: 
        The name of the new field created by the decoder. The decoder will output a JSON object with the following elements:

        - latitute: string,
        - longitude: string,
        - location: [ float64, float64 ],
                - GeoJSON format intended for use as a `geo_point <http://www.elasticsearch.org/guide/en/elasticsearch/reference/current/mapping-geo-point-type.html/>`_ for ES output.
                  Useful when using Kibana's `Bettermap panel <http://www.elasticsearch.org/guide/en/elasticsearch/reference/current/mapping-geo-point-type.html http://www.elasticsearch.org/guide/en/kibana/current/_bettermap.html/>`_
        - coordinates: [ string, string ],
        - countrycode: string,
        - countrycode3: string,
        - region: string,
        - city: string,
        - postalcode: string,
        - areacode: int,
        - charset: int,
        - continentalcode: string

.. code-block:: ini

    [apache_geoip_decoder]
    type = "GeoIpDecoder"
    db_file="/etc/geoip/GeoLiteCity.dat"
    source_ip_field="remote_host"
    target_field="geoip"

