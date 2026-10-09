const test = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const core = require('./html/rpc/hiddify_grpc_web_pb.js');
const ext = require('./html/rpc/extension_grpc_web_pb.js');
test('browser bundle initializes its page entry point', () => {
  const context = {window: {}, console, setTimeout, clearTimeout, Uint8Array, ArrayBuffer, TextEncoder, TextDecoder};
  vm.runInNewContext(fs.readFileSync(__dirname + '/html/rpc.js', 'utf8'), context);
  assert.equal(typeof context.window.onload, 'function');
});
test('current StartRequest preserves Unicode through binary serialization', () => {
  const request = new core.StartRequest();
  request.setConfigContent('тест 🦊');
  request.setEnableRawConfig(true);
  const result = core.StartRequest.deserializeBinary(request.serializeBinary());
  assert.equal(result.getConfigContent(), 'тест 🦊');
  assert.equal(result.getEnableRawConfig(), true);
});
test('service clients use current protobuf service paths', async () => {
  const c = new core.CorePromiseClient('/');
  const e = new ext.ExtensionHostServicePromiseClient('/');
  const paths = [];
  for (const client of [c, e]) client.client_.unaryCall = (path, request, metadata, descriptor) => {paths.push(path); return Promise.resolve({});};
  await c.start(new core.StartRequest(), {});
  await e.listExtensions(new ext.Empty(), {});
  assert.deepEqual(paths, ['/hcore.Core/Start', '/extension.ExtensionHostService/ListExtensions']);
});
test('streamed core state updates the connection indicator', () => {
  const clientModule = require('./html/rpc/client.js');
  let handler;
  clientModule.hiddifyClient.coreInfoListener = () => ({on(event, callback) {if (event === 'data') handler = callback; return this;}});
  const labels = new Map();
  global.$ = selector => ({show() {}, hide() {}, click() {}, css() {}, text(value) {labels.set(selector, value);}});
  try {
    require('./html/rpc/connectionPage.js').openConnectionPage();
    const response = new core.CoreInfoResponse();
    response.setCoreState(core.CoreState.STARTED);
    handler(response);
    assert.equal(labels.get('#connection-status'), 'Connected');
  } finally {delete global.$;}
});
