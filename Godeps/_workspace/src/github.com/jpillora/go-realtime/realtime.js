/* global window,WebSocket */
// Realtime - v0.1.0 - https://github.com/jpillora/go-realtime
// Jaime Pillora <dev@jpillora.com> - MIT Copyright 2015
(function(window, document) {

  if(!window.WebSocket)
    return alert("This browser does not support WebSockets");

  //realtime protocol version
  var proto = "v1";

  //public method
  var realtime = function(url) {
    var rt = new Realtime(url);
    rts.push(rt);
    return rt;
  };
  realtime.proto = proto;
  realtime.online = true;

  //special merge - ignore $properties
  // x <- y
  var merge = function(x, y) {
    if (!x || typeof x !== "object" ||
      !y || typeof y !== "object")
      return y;
    var k;
    if (x instanceof Array && y instanceof Array)
      while (x.length > y.length)
        x.pop();
    else
      for (k in x)
        if (k[0] !== "$" && !(k in y))
          delete x[k];
    for (k in y)
      x[k] = merge(x[k], y[k]);
    return x;
  };

  var rts = [];
  //global status change handler
  function onstatus(event) {
    realtime.online = navigator.onLine;
    for(var i = 0; i < rts.length; i++) {
      if(realtime.online && rts[i].autoretry)
        rts[i].retry();
    }
  }
  window.addEventListener('online',  onstatus);
  window.addEventListener('offline', onstatus);

  //helpers
  var events = ["message","error","open","close"];
  var loc = window.location;
  //Realtime class - represents a single websocket (User on the server-side)
  function Realtime(url) {
    if(!url)
      url = "/realtime";
    if(!(/^https?:/.test(url)))
      url = loc.protocol + "//" + loc.host + url;
    if(!(/^http(s?:\/\/.+)$/.test(url)))
      throw "Invalid URL: " + url;
    this.url = "ws" + RegExp.$1;
    this.connect();
    this.objs = {};
    this.subs = {};
    this.onupdates = {};
    this.connected = false;
  }
  Realtime.prototype = {
    add: function(key, obj, onupdate) {
      if(typeof key !== "string")
        throw "Invalid key - must be string";
      if(!obj || typeof obj !== "object")
        throw "Invalid object - must be an object";
      if(this.objs[key])
        throw "Duplicate key - already added";
      this.objs[key] = obj;
      this.subs[key] = 0;
      this.onupdates[key] = onupdate;
      this.subscribe();
    },
    connect: function() {
      this.autoretry = true;
      this.retry();
    },
    retry: function() {
      clearTimeout(this.retry.t);
      if(this.ws)
        this.cleanup();
      if(!this.delay)
        this.delay = 100;
      this.ws = new WebSocket(this.url);
      var _this = this;
      events.forEach(function(e) {
        e = "on"+e;
        _this.ws[e] = _this[e].bind(_this);
      });
      this.ping.t = setInterval(this.ping.bind(this), 30 * 1000);
    },
    disconnect: function() {
      this.autoretry = false;
      this.cleanup();
    },
    cleanup: function(){
      if(!this.ws)
        return;
      var _this = this;
      events.forEach(function(e) {
        _this.ws["on"+e] = null;
      });
      if(this.ws.readyState !== WebSocket.CLOSED)
        this.ws.close();
      this.ws = null;
      clearInterval(this.ping.t);
    },
    send: function(data) {
      if(this.ws.readyState === WebSocket.OPEN)
        return this.ws.send(data);
    },
    ping: function() {
      this.send("ping");
    },
    subscribe: function() {
      this.send(JSON.stringify({
        protocol: proto,
        objectVersions: this.subs
      }));
    },
    onmessage: function(event) {
      var str = event.data;
      if (str === "ping") return;

      var updates;
      try {
        updates = JSON.parse(str);
      } catch(err) {
        return console.warn(err, str);
      }

      for(var i = 0; i < updates.length; i++) {
        var u = updates[i];
        var key = u.Key;
        var dst = this.objs[key];
        var src = u.Data;
        if(!src || !dst)
          continue;

        if(u.Delta)
          jsonpatch.apply(dst, src);
        else
          merge(dst, src);

        if(typeof dst.$apply === "function")
          dst.$apply();

        var onupdate = this.onupdates[key];
        if(typeof onupdate === "function")
          onupdate();

        this.subs[key] = u.Version;
      }
      //successful msg resets retry counter
      this.delay = 100;
    },
    onopen: function() {
      this.connected = true;
      if(this.onstatus) this.onstatus(true);
      this.subscribe();
    },
    onclose: function() {
      this.connected = false;
      if(this.onstatus) this.onstatus(false);
      this.delay *= 2;
      if(this.autoretry) {
        this.retry.t = setTimeout(this.connect.bind(this), this.delay);
      }
    },
    onerror: function(err) {
      // console.error("websocket error: %s", err);
    }
  };
  //publicise
  window.realtime = realtime;
}(window, document, undefined));

/*!
* https://github.com/Starcounter-Jack/JSON-Patch
* json-patch-duplex.js version: 0.5.4
* (c) 2013 Joachim Wester
* MIT license
*/
var __extends = this.__extends || function (d, b) {
    for (var p in b) if (b.hasOwnProperty(p)) d[p] = b[p];
    function __() { this.constructor = d; }
    __.prototype = b.prototype;
    d.prototype = new __();
};
var OriginalError = Error;

var jsonpatch;
(function (jsonpatch) {
    /* Do nothing if module is already defined.
    Doesn't look nice, as we cannot simply put
    `!jsonpatch &&` before this immediate function call
    in TypeScript.
    */
    if (jsonpatch.apply) {
        return;
    }

    var _objectKeys = (function () {
        if (Object.keys)
            return Object.keys;

        return function (o) {
            var keys = [];
            for (var i in o) {
                if (o.hasOwnProperty(i)) {
                    keys.push(i);
                }
            }
            return keys;
        };
    })();

    function _equals(a, b) {
        switch (typeof a) {
            case 'undefined':
            case 'boolean':
            case 'string':
            case 'number':
                return a === b;
            case 'object':
                if (a === null)
                    return b === null;
                if (_isArray(a)) {
                    if (!_isArray(b) || a.length !== b.length)
                        return false;

                    for (var i = 0, l = a.length; i < l; i++)
                        if (!_equals(a[i], b[i]))
                            return false;

                    return true;
                }

                var bKeys = _objectKeys(b);
                var bLength = bKeys.length;
                if (_objectKeys(a).length !== bLength)
                    return false;

                for (var i = 0; i < bLength; i++)
                    if (!_equals(a[i], b[i]))
                        return false;

                return true;

            default:
                return false;
        }
    }

    /* We use a Javascript hash to store each
    function. Each hash entry (property) uses
    the operation identifiers specified in rfc6902.
    In this way, we can map each patch operation
    to its dedicated function in efficient way.
    */
    /* The operations applicable to an object */
    var objOps = {
        add: function (obj, key) {
            obj[key] = this.value;
            return true;
        },
        remove: function (obj, key) {
            delete obj[key];
            return true;
        },
        replace: function (obj, key) {
            obj[key] = this.value;
            return true;
        },
        move: function (obj, key, tree) {
            var temp = { op: "_get", path: this.from };
            apply(tree, [temp]);
            apply(tree, [
                { op: "remove", path: this.from }
            ]);
            apply(tree, [
                { op: "add", path: this.path, value: temp.value }
            ]);
            return true;
        },
        copy: function (obj, key, tree) {
            var temp = { op: "_get", path: this.from };
            apply(tree, [temp]);
            apply(tree, [
                { op: "add", path: this.path, value: temp.value }
            ]);
            return true;
        },
        test: function (obj, key) {
            return _equals(obj[key], this.value);
        },
        _get: function (obj, key) {
            this.value = obj[key];
        }
    };

    /* The operations applicable to an array. Many are the same as for the object */
    var arrOps = {
        add: function (arr, i) {
            arr.splice(i, 0, this.value);
            return true;
        },
        remove: function (arr, i) {
            arr.splice(i, 1);
            return true;
        },
        replace: function (arr, i) {
            arr[i] = this.value;
            return true;
        },
        move: objOps.move,
        copy: objOps.copy,
        test: objOps.test,
        _get: objOps._get
    };

    /* The operations applicable to object root. Many are the same as for the object */
    var rootOps = {
        add: function (obj) {
            rootOps.remove.call(this, obj);
            for (var key in this.value) {
                if (this.value.hasOwnProperty(key)) {
                    obj[key] = this.value[key];
                }
            }
            return true;
        },
        remove: function (obj) {
            for (var key in obj) {
                if (obj.hasOwnProperty(key)) {
                    objOps.remove.call(this, obj, key);
                }
            }
            return true;
        },
        replace: function (obj) {
            apply(obj, [
                { op: "remove", path: this.path }
            ]);
            apply(obj, [
                { op: "add", path: this.path, value: this.value }
            ]);
            return true;
        },
        move: objOps.move,
        copy: objOps.copy,
        test: function (obj) {
            return (JSON.stringify(obj) === JSON.stringify(this.value));
        },
        _get: function (obj) {
            this.value = obj;
        }
    };

    var _isArray;
    if (Array.isArray) {
        _isArray = Array.isArray;
    } else {
        _isArray = function (obj) {
            return obj.push && typeof obj.length === 'number';
        };
    }

    //3x faster than cached /^\d+$/.test(str)
    function isInteger(str) {
        var i = 0;
        var len = str.length;
        var charCode;
        while (i < len) {
            charCode = str.charCodeAt(i);
            if (charCode >= 48 && charCode <= 57) {
                i++;
                continue;
            }
            return false;
        }
        return true;
    }

    /// Apply a json-patch operation on an object tree
    function apply(tree, patches, validate) {
        var result = false, p = 0, plen = patches.length, patch, key;
        while (p < plen) {
            patch = patches[p];
            p++;

            // Find the object
            var path = patch.path || "";
            var keys = path.split('/');
            var obj = tree;

            var t = 1;
            var len = keys.length;
            var existingPathFragment = undefined;

            while (true) {
                key = keys[t];

                if (validate) {
                    if (existingPathFragment === undefined) {
                        if (obj[key] === undefined) {
                            existingPathFragment = keys.slice(0, t).join('/');
                        } else if (t == len - 1) {
                            existingPathFragment = patch.path;
                        }
                        if (existingPathFragment !== undefined) {
                            this.validator(patch, p - 1, tree, existingPathFragment);
                        }
                    }
                }

                t++;
                if (key === undefined) {
                    if (t >= len) {
                        result = rootOps[patch.op].call(patch, obj, key, tree); // Apply patch
                        break;
                    }
                }
                if (_isArray(obj)) {
                    if (key === '-') {
                        key = obj.length;
                    } else {
                        if (validate && !isInteger(key)) {
                            throw new JsonPatchError("Expected an unsigned base-10 integer value, making the new referenced value the array element with the zero-based index", "OPERATION_PATH_ILLEGAL_ARRAY_INDEX", p - 1, patch.path, patch);
                        }
                        key = parseInt(key, 10);
                    }
                    if (t >= len) {
                        if (validate && patch.op === "add" && key > obj.length) {
                            throw new JsonPatchError("The specified index MUST NOT be greater than the number of elements in the array", "OPERATION_VALUE_OUT_OF_BOUNDS", p - 1, patch.path, patch);
                        }
                        result = arrOps[patch.op].call(patch, obj, key, tree); // Apply patch
                        break;
                    }
                } else {
                    if (key && key.indexOf('~') != -1)
                        key = key.replace(/~1/g, '/').replace(/~0/g, '~'); // escape chars
                    if (t >= len) {
                        result = objOps[patch.op].call(patch, obj, key, tree); // Apply patch
                        break;
                    }
                }
                if(!obj) {
                  console.warning("slashes in object keys not supported");
                  break;
                }
                obj = obj[key];
            }
        }
        return result;
    }
    jsonpatch.apply = apply;

    var JsonPatchError = (function (_super) {
        __extends(JsonPatchError, _super);
        function JsonPatchError(message, name, index, operation, tree) {
            _super.call(this, message);
            this.message = message;
            this.name = name;
            this.index = index;
            this.operation = operation;
            this.tree = tree;
        }
        return JsonPatchError;
    })(OriginalError);
    jsonpatch.JsonPatchError = JsonPatchError;

    jsonpatch.Error = JsonPatchError;

    /**
    * Recursively checks whether an object has any undefined values inside.
    */
    function hasUndefined(obj) {
        if (obj === undefined) {
            return true;
        }

        if (typeof obj == "array" || typeof obj == "object") {
            for (var i in obj) {
                if (hasUndefined(obj[i])) {
                    return true;
                }
            }
        }

        return false;
    }

    /**
    * Validates a single operation. Called from `jsonpatch.validate`. Throws `JsonPatchError` in case of an error.
    * @param {object} operation - operation object (patch)
    * @param {number} index - index of operation in the sequence
    * @param {object} [tree] - object where the operation is supposed to be applied
    * @param {string} [existingPathFragment] - comes along with `tree`
    */
    function validator(operation, index, tree, existingPathFragment) {
        if (typeof operation !== 'object' || operation === null || _isArray(operation)) {
            throw new JsonPatchError('Operation is not an object', 'OPERATION_NOT_AN_OBJECT', index, operation, tree);
        } else if (!objOps[operation.op]) {
            throw new JsonPatchError('Operation `op` property is not one of operations defined in RFC-6902', 'OPERATION_OP_INVALID', index, operation, tree);
        } else if (typeof operation.path !== 'string') {
            throw new JsonPatchError('Operation `path` property is not a string', 'OPERATION_PATH_INVALID', index, operation, tree);
        } else if ((operation.op === 'move' || operation.op === 'copy') && typeof operation.from !== 'string') {
            throw new JsonPatchError('Operation `from` property is not present (applicable in `move` and `copy` operations)', 'OPERATION_FROM_REQUIRED', index, operation, tree);
        } else if ((operation.op === 'add' || operation.op === 'replace' || operation.op === 'test') && operation.value === undefined) {
            throw new JsonPatchError('Operation `value` property is not present (applicable in `add`, `replace` and `test` operations)', 'OPERATION_VALUE_REQUIRED', index, operation, tree);
        } else if ((operation.op === 'add' || operation.op === 'replace' || operation.op === 'test') && hasUndefined(operation.value)) {
            throw new JsonPatchError('Operation `value` property is not present (applicable in `add`, `replace` and `test` operations)', 'OPERATION_VALUE_CANNOT_CONTAIN_UNDEFINED', index, operation, tree);
        } else if (tree) {
            if (operation.op == "add") {
                var pathLen = operation.path.split("/").length;
                var existingPathLen = existingPathFragment.split("/").length;
                if (pathLen !== existingPathLen + 1 && pathLen !== existingPathLen) {
                    throw new JsonPatchError('Cannot perform an `add` operation at the desired path', 'OPERATION_PATH_CANNOT_ADD', index, operation, tree);
                }
            } else if (operation.op === 'replace' || operation.op === 'remove' || operation.op === '_get') {
                if (operation.path !== existingPathFragment) {
                    throw new JsonPatchError('Cannot perform the operation at a path that does not exist', 'OPERATION_PATH_UNRESOLVABLE', index, operation, tree);
                }
            } else if (operation.op === 'move' || operation.op === 'copy') {
                var existingValue = { op: "_get", path: operation.from, value: undefined };
                var error = jsonpatch.validate([existingValue], tree);
                if (error && error.name === 'OPERATION_PATH_UNRESOLVABLE') {
                    throw new JsonPatchError('Cannot perform the operation from a path that does not exist', 'OPERATION_FROM_UNRESOLVABLE', index, operation, tree);
                }
            }
        }
    }
    jsonpatch.validator = validator;

    /**
    * Validates a sequence of operations. If `tree` parameter is provided, the sequence is additionally validated against the object tree.
    * If error is encountered, returns a JsonPatchError object
    * @param sequence
    * @param tree
    * @returns {JsonPatchError|undefined}
    */
    function validate(sequence, tree) {
        try  {
            if (!_isArray(sequence)) {
                throw new JsonPatchError('Patch sequence must be an array', 'SEQUENCE_NOT_AN_ARRAY');
            }

            if (tree) {
                tree = JSON.parse(JSON.stringify(tree)); //clone tree so that we can safely try applying operations
                apply.call(this, tree, sequence, true);
            } else {
                for (var i = 0; i < sequence.length; i++) {
                    this.validator(sequence[i], i);
                }
            }
        } catch (e) {
            if (e instanceof JsonPatchError) {
                return e;
            } else {
                throw e;
            }
        }
    }
    jsonpatch.validate = validate;
})(jsonpatch || (jsonpatch = {}));

if (typeof exports !== "undefined") {
    exports.apply = jsonpatch.apply;
    exports.validate = jsonpatch.validate;
    exports.validator = jsonpatch.validator;
    exports.JsonPatchError = jsonpatch.JsonPatchError;
    exports.Error = jsonpatch.Error;
}
