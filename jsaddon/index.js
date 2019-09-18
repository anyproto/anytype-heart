const bindings = require( 'bindings' )( 'addon' );
var protobuf = require( "protobufjs" );
var SegfaultHandler = require( 'segfault-handler' );

SegfaultHandler.registerHandler( "crash.log" );
var GenerateMnemonicMessage;
var PrintMnemonicMessage;
function buf2hex(buffer) { // buffer is an ArrayBuffer
	return Array.prototype.map.call(new Uint8Array(buffer), x => ('00' + x.toString(16)).slice(-2)).join('');
}

bindings.setCallback( item => {
	//console.log("go from go: "+buf2hex(item.data));
//	var message = GenerateMnemonicMessage.decode( item.data );
	try {
		var message = PrintMnemonicMessage.decode( toBuffer(item.data) );
		console.log("got mnemonic: %s", message.mnemonic);
	}
	catch(err) {
		console.log(err);
	}

	/* // Answer the call with a 90% probability of returning true somewhere between
	 // 200 and 400 ms from now.
	 setTimeout(() => {
	   const theAnswer = (Math.random() > 0.1);
	   console.log(thePrime + ': answering with ' + theAnswer);
	   bindings.registerReturnValue(item, theAnswer);
	 }, Math.random() * 200 + 200);*/
} );

function ab2str(buf){
	return String.fromCharCode.apply( null, new Uint8Array( buf ) );
}

function str2ab(str){
	var buf = new ArrayBuffer( str.length ); // 1 bytes for each char
	var bufView = new Uint8Array( buf );
	for (var i = 0, strLen = str.length; i < strLen; i++) {
		bufView[i] = str.charCodeAt( i );
	}
	return buf;
}

var a = str2ab( "bbb" );

//bindings.callMethod("Generatefff", a);
//bindings.callMethod("a", "ff");

//console.log(a);
function toArrayBuffer(buffer){
	var ab = new ArrayBuffer( buffer.length );
	var view = new Uint8Array( ab );
	for (var i = 0; i < buffer.length; ++i) {
		view[i] = buffer[i];
	}
	return ab;
}

function toBuffer(ab) {
	var buffer = new Buffer(ab.byteLength);
	var view = new Uint8Array(ab);
	for (var i = 0; i < buffer.length; ++i) {
		buffer[i] = view[i];
	}
	return buffer;
}

protobuf.load( "../pb/protos/service.proto", function (err, root){
	if (err)
		throw err;
	// Obtain a message type
	GenerateMnemonicMessage = root.lookupType( "anytype.GenerateMnemonic" );
	PrintMnemonicMessage = root.lookupType( "anytype.PrintMnemonic" );
	
	var hex = 'AA5504B10000B5'
	
	var buffer = GenerateMnemonicMessage.encode( {wordsCount: 12} ).finish();
	
	//var m = GenerateMnemonicMessage.decode( buffer);
	
	console.log("---", buffer.toString('hex'));
	bindings.callMethod( "GenerateMnemonic", toArrayBuffer(buffer) );
	
	//console.log(buffer);
	//  console.log(str2ab("aa"));
} );

setTimeout( () => {
	console.log( a );
	console.log( '...' );
}, 5000 );

/*
var start = new Date().getTime();

for (i = 0; i < 10; ++i) {
    bindings.callMethod("GenerateMnemonic", str2ab("b"));
}

var end = new Date().getTime();
var time = end - start;
console.log('Execution time: ' + time);

setTimeout(() => {
  console.log('...');
}, 20000);
*/
