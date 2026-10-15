// Runs before the scripts of every new document and frame (B226). It makes
// headless Chrome look like the Chrome that people use. Every item has its
// own try, so one failure never breaks the page or the other items. The file
// is one function expression without a semicolon; Go adds the call with the
// settings:
// {languages: ["zh-CN", "zh"], webglVendor, webglRenderer, chromeBarHeight}.
(function (cfg) {
  'use strict';

  // ---- native-looking functions ----
  // Functions that this script makes print as native code when someone calls
  // Function.prototype.toString on them.
  var native = new WeakMap();
  function mask(fn, name) {
    try {
      Object.defineProperty(fn, 'name', { value: name, configurable: true });
    } catch (e) {}
    native.set(fn, 'function ' + name + '() { [native code] }');
    return fn;
  }

  try {
    var origToString = Function.prototype.toString;
    var toStringProxy = new Proxy(origToString, {
      apply: function (target, thisArg, args) {
        if (thisArg !== null && (typeof thisArg === 'function' || typeof thisArg === 'object') && native.has(thisArg)) {
          return native.get(thisArg);
        }
        return Reflect.apply(target, thisArg, args);
      },
    });
    native.set(toStringProxy, 'function toString() { [native code] }');
    Object.defineProperty(Function.prototype, 'toString', {
      value: toStringProxy,
      writable: true,
      configurable: true,
      enumerable: false,
    });
  } catch (e) {}

  // Replace the getter of an accessor on proto. fn runs with the page's this.
  function defineGetter(proto, prop, fn) {
    var old = Object.getOwnPropertyDescriptor(proto, prop);
    var get = mask(fn, 'get ' + prop);
    Object.defineProperty(proto, prop, {
      get: get,
      set: old ? old.set : undefined,
      enumerable: old ? old.enumerable : true,
      configurable: true,
    });
    return old;
  }

  // ---- navigator.webdriver ----
  try {
    defineGetter(Navigator.prototype, 'webdriver', function () {
      return false;
    });
  } catch (e) {}

  // ---- navigator.languages ----
  try {
    var langs = Object.freeze(cfg.languages.slice());
    defineGetter(Navigator.prototype, 'languages', function () {
      return langs;
    });
    defineGetter(Navigator.prototype, 'language', function () {
      return langs[0];
    });
  } catch (e) {}

  // ---- navigator.plugins and navigator.mimeTypes ----
  // The four classes have getters that only work on real objects. The fake
  // objects are registered in `fakes`, and the getters and methods of the
  // classes answer for them and pass everything else to the real ones.
  try {
    if (navigator.plugins.length !== 5) {
      var fakes = new WeakMap();
      var wrapGetter = function (proto, prop) {
        var old = Object.getOwnPropertyDescriptor(proto, prop);
        var orig = old.get;
        var get = mask(function () {
          var f = fakes.get(this);
          return f ? f[prop] : orig.call(this);
        }, 'get ' + prop);
        Object.defineProperty(proto, prop, { get: get, set: old.set, enumerable: old.enumerable, configurable: true });
      };
      var wrapMethod = function (proto, name, make) {
        var old = Object.getOwnPropertyDescriptor(proto, name);
        var orig = old.value;
        var fn = make(function (self) {
          return fakes.get(self);
        }, orig);
        Object.defineProperty(fn, 'length', { value: orig.length, configurable: true });
        mask(fn, name);
        Object.defineProperty(proto, name, { value: fn, writable: old.writable, enumerable: old.enumerable, configurable: true });
      };
      var lookup = function (list, key) {
        // Like item and namedItem: null when there is nothing.
        return list[key] || null;
      };
      var hide = function (obj, key, value, enumerable) {
        Object.defineProperty(obj, key, { value: value, enumerable: !!enumerable, configurable: true, writable: false });
      };

      wrapGetter(MimeType.prototype, 'type');
      wrapGetter(MimeType.prototype, 'suffixes');
      wrapGetter(MimeType.prototype, 'description');
      wrapGetter(MimeType.prototype, 'enabledPlugin');
      wrapGetter(Plugin.prototype, 'name');
      wrapGetter(Plugin.prototype, 'filename');
      wrapGetter(Plugin.prototype, 'description');
      wrapGetter(Plugin.prototype, 'length');
      wrapGetter(PluginArray.prototype, 'length');
      wrapGetter(MimeTypeArray.prototype, 'length');
      [Plugin.prototype, PluginArray.prototype, MimeTypeArray.prototype].forEach(function (proto) {
        wrapMethod(proto, 'item', function (data, orig) {
          return function (index) {
            var f = data(this);
            return f ? lookup(f.byIndex, index >>> 0) : orig.apply(this, arguments);
          };
        });
        wrapMethod(proto, 'namedItem', function (data, orig) {
          return function (name) {
            var f = data(this);
            return f ? lookup(f.byName, String(name)) : orig.apply(this, arguments);
          };
        });
      });
      wrapMethod(PluginArray.prototype, 'refresh', function (data, orig) {
        return function () {
          if (data(this)) return undefined;
          return orig.apply(this, arguments);
        };
      });

      var mimeSpecs = [
        { type: 'application/pdf', suffixes: 'pdf', description: 'Portable Document Format' },
        { type: 'text/pdf', suffixes: 'pdf', description: 'Portable Document Format' },
      ];
      var pluginNames = ['PDF Viewer', 'Chrome PDF Viewer', 'Chromium PDF Viewer', 'Microsoft Edge PDF Viewer', 'WebKit built-in PDF'];

      // An array-like: indexed items are enumerable, named ones are not.
      var fill = function (arr, items, nameOf) {
        var data = { byIndex: Object.create(null), byName: Object.create(null), length: items.length };
        items.forEach(function (it, i) {
          data.byIndex[i] = it;
          data.byName[nameOf(it)] = it;
          hide(arr, i, it, true);
        });
        items.forEach(function (it) {
          if (!(nameOf(it) in arr)) hide(arr, nameOf(it), it, false);
        });
        return data;
      };

      var mimes = mimeSpecs.map(function (s) {
        var m = Object.create(MimeType.prototype);
        fakes.set(m, { type: s.type, suffixes: s.suffixes, description: s.description, enabledPlugin: null });
        return m;
      });
      var plugins = pluginNames.map(function (n) {
        var p = Object.create(Plugin.prototype);
        var d = fill(p, mimes, function (m) {
          return fakes.get(m).type;
        });
        d.name = n;
        d.filename = 'internal-pdf-viewer';
        d.description = 'Portable Document Format';
        fakes.set(p, d);
        return p;
      });
      mimes.forEach(function (m) {
        fakes.get(m).enabledPlugin = plugins[0];
      });
      var mimeArray = Object.create(MimeTypeArray.prototype);
      fakes.set(
        mimeArray,
        fill(mimeArray, mimes, function (m) {
          return fakes.get(m).type;
        })
      );
      var pluginArray = Object.create(PluginArray.prototype);
      fakes.set(
        pluginArray,
        fill(pluginArray, plugins, function (p) {
          return fakes.get(p).name;
        })
      );
      defineGetter(Navigator.prototype, 'plugins', function () {
        return pluginArray;
      });
      defineGetter(Navigator.prototype, 'mimeTypes', function () {
        return mimeArray;
      });
    }
  } catch (e) {}

  // ---- window.chrome ----
  try {
    var def = function (obj, key, value) {
      if (!(key in obj)) {
        Object.defineProperty(obj, key, { value: value, writable: true, enumerable: true, configurable: true });
      }
    };
    if (typeof window.chrome !== 'object' || window.chrome === null) {
      Object.defineProperty(window, 'chrome', { value: {}, writable: true, enumerable: false, configurable: true });
    }
    var chrome = window.chrome;
    var timing = function () {
      return (window.performance && performance.timing) || {};
    };
    def(
      chrome,
      'loadTimes',
      mask(function () {
        var t = timing();
        var start = (t.navigationStart || Date.now()) / 1000;
        return {
          requestTime: start,
          startLoadTime: start,
          commitLoadTime: (t.responseStart || t.navigationStart || Date.now()) / 1000,
          finishDocumentLoadTime: (t.domContentLoadedEventEnd || 0) / 1000,
          finishLoadTime: (t.loadEventEnd || 0) / 1000,
          firstPaintTime: start,
          firstPaintAfterLoadTime: 0,
          navigationType: 'Other',
          wasFetchedViaSpdy: true,
          wasNpnNegotiated: true,
          npnNegotiatedProtocol: 'h2',
          wasAlternateProtocolAvailable: false,
          connectionInfo: 'h2',
        };
      }, 'loadTimes')
    );
    def(
      chrome,
      'csi',
      mask(function () {
        var t = timing();
        return {
          onloadT: t.domContentLoadedEventEnd || Date.now(),
          startE: t.navigationStart || Date.now(),
          pageT: Date.now() - (t.navigationStart || Date.now()),
          tran: 15,
        };
      }, 'csi')
    );
    def(chrome, 'app', {
      isInstalled: false,
      InstallState: { DISABLED: 'disabled', INSTALLED: 'installed', NOT_INSTALLED: 'not_installed' },
      RunningState: { CANNOT_RUN: 'cannot_run', READY_TO_RUN: 'ready_to_run', RUNNING: 'running' },
      getDetails: mask(function () {
        return null;
      }, 'getDetails'),
      getIsInstalled: mask(function () {
        return false;
      }, 'getIsInstalled'),
      runningState: mask(function () {
        return 'cannot_run';
      }, 'runningState'),
    });
    def(chrome, 'runtime', {
      OnInstalledReason: { CHROME_UPDATE: 'chrome_update', INSTALL: 'install', SHARED_MODULE_UPDATE: 'shared_module_update', UPDATE: 'update' },
      OnRestartRequiredReason: { APP_UPDATE: 'app_update', OS_UPDATE: 'os_update', PERIODIC: 'periodic' },
      PlatformArch: { ARM: 'arm', ARM64: 'arm64', MIPS: 'mips', MIPS64: 'mips64', X86_32: 'x86-32', X86_64: 'x86-64' },
      PlatformOs: { ANDROID: 'android', CROS: 'cros', LINUX: 'linux', MAC: 'mac', OPENBSD: 'openbsd', WIN: 'win' },
      RequestUpdateCheckStatus: { NO_UPDATE: 'no_update', THROTTLED: 'throttled', UPDATE_AVAILABLE: 'update_available' },
      connect: mask(function () {
        throw new TypeError('Error in invocation of runtime.connect(optional string extensionId, optional object connectInfo): chrome.runtime.connect() called from a webpage must specify an Extension ID (string) for its first argument.');
      }, 'connect'),
      sendMessage: mask(function () {
        throw new TypeError('Error in invocation of runtime.sendMessage(optional string extensionId, any message, optional object options, optional function callback): chrome.runtime.sendMessage() called from a webpage must specify an Extension ID (string) for its first argument.');
      }, 'sendMessage'),
    });
  } catch (e) {}

  // ---- Notification.permission and permissions.query ----
  try {
    var origQuery = Permissions.prototype.query;
    var query = mask(function (desc) {
      var p = origQuery.apply(this, arguments);
      if (!desc || desc.name !== 'notifications' || typeof Notification === 'undefined') return p;
      return p.then(function (status) {
        try {
          var perm = Notification.permission;
          var want = perm === 'default' ? 'prompt' : perm;
          if (status.state !== want) {
            Object.defineProperty(status, 'state', { value: want, enumerable: true, configurable: true });
          }
        } catch (e) {}
        return status;
      });
    }, 'query');
    Object.defineProperty(query, 'length', { value: 1, configurable: true });
    Object.defineProperty(Permissions.prototype, 'query', { value: query, writable: true, enumerable: true, configurable: true });
  } catch (e) {}

  // ---- outerWidth and outerHeight ----
  try {
    var outer = function (prop, inner, extra) {
      var old = Object.getOwnPropertyDescriptor(window, prop);
      var get = mask(function () {
        var w = window;
        try {
          if (window.top && window.top.innerWidth) w = window.top;
        } catch (e) {}
        return w[inner] + extra;
      }, 'get ' + prop);
      var set = old && old.set ? old.set : mask(function (v) {
        Object.defineProperty(window, prop, { value: v, writable: true, enumerable: true, configurable: true });
      }, 'set ' + prop);
      Object.defineProperty(window, prop, { get: get, set: set, enumerable: true, configurable: true });
    };
    outer('outerWidth', 'innerWidth', 0);
    outer('outerHeight', 'innerHeight', cfg.chromeBarHeight);
  } catch (e) {}

  // ---- WebGL vendor and renderer ----
  try {
    [window.WebGLRenderingContext, window.WebGL2RenderingContext].forEach(function (Ctx) {
      if (!Ctx) return;
      var orig = Ctx.prototype.getParameter;
      var getParameter = mask(function (pname) {
        var v = orig.apply(this, arguments);
        // Only when the page got the real answer, so the errors of a context
        // without the debug extension stay the same.
        if (v !== null && v !== undefined) {
          if (pname === 0x9245) return cfg.webglVendor; // UNMASKED_VENDOR_WEBGL
          if (pname === 0x9246) return cfg.webglRenderer; // UNMASKED_RENDERER_WEBGL
        }
        return v;
      }, 'getParameter');
      Object.defineProperty(getParameter, 'length', { value: 1, configurable: true });
      Object.defineProperty(Ctx.prototype, 'getParameter', { value: getParameter, writable: true, enumerable: true, configurable: true });
    });
  } catch (e) {}
})
