const common = require('./v2/hcommon/common_pb.js');
const messages = require('./v2/hcore/hcore_pb.js');
module.exports = {...common, ...messages, ...require('./v2/hcore/hcore_service_grpc_web_pb.js'), CoreState: messages.CoreStates};
