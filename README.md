goconfd
=======

A simple configuration server (ready-to-download 0-dependency binaries included).

This is sort of a combination of [etcd](https://github.com/coreos/etcd) [1] and [confd](https://github.com/kelseyhightower/confd) [2].

[1] without a lot of stuff that may or may not be usefull to you.
[2] without more stuff that may or may not be usefull to you.

goconfd features:

* A simple JSON serving HTTP server
* deep link capabities
* integrated templating engine (just go templates for now; request others if you have a need for that)
* blocking requests

Or in terms of HTTP verbs:

* GET retrieves JSON
* POST retrieves a filled out template
* PUT unblocks blocking requests

Each one of these supports deeplinks.

Simple example
--------------

You start goconfd with

```
goconfd conf.json
```

will start the goconfd server on port 6666. You can tell it to use another port with the @--port@ flag.

Given a conf.json like this:


```json
{
	"mysql": {
		"master": {
			"host": "mysql.mysite",
			"port": 3306,
			"username": "myuser",
			"password": "s3cr3t"
		},
		"slave": {
			"host": "mysql.mysite",
			"port": 3307,
			"username": "myuser",
			"password": "s3cr3t"
		}
	}
}
```

Retrieving JSON configurations
------------------------------

you can get the whole configuration with a simple HTTP request:

```
curl localhost:6666
```

and just the mysql master configuration with a deep link:


```
curl localhost:6666/mysql/master
```

Posting configuration templates
-------------------------------

You can also use a template like this

```
Hostname: {{.host}}
Port: {{.port}}
Username: {{.username}}
Pass: {{.passwort}}
```

and POST this template to goconfd:

```
curl --data-binary @myconf.template localhost/mysql/master
```

to make goconfd fill out your template. Note that when POSTing deeplinks work, too.

goconfd uses [golang templates](http://golang.org/pkg/text/template/). There are some helpers defined. This is a part I'm not really happy with as it is hard to generalize template helpers. So far there are:

* path_join
* split
* join
* add
* first
* last
* now
* today

Usage sample (the beginning of an NGINX configuration):

```json
user {{.user}};
worker_processes {{len .child_nodes | add 1}};

daemon on;

error_log {{path_join .log_dir .server}}.error.log;
pid       {{path_join .pid_dir .server}}.pid;

events {
  worker_connections 1024;
}

...
```

Blocking requests
-----------------

Issuing a GET or POST request with a @wait@ query parameter makes goconfd block the request until it is unblocked by a PUT request to the corresponding URL. The idea is that you write a start/stop script for your service that configures and starts the service. Then it ussues a blocking request. When the request is returned, the process told to reload the new configuration.


FAQ
---

* Why not mustache?
  In my first attempts to port even simple configurations from chef to goconfd I realized that the template engine must support at least some helpers. With Mustache helpers are bound to the values which are filled into the template. As in goconfd the values are just what's supported by JSON I see no way who helpers should be possible. I'd like to be proven wrong on this.
* Can I have helper X?
  Sure. Post a github issue. Or even better fork & pull request.

TODO
----

* IPv6 support

