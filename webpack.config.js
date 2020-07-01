var path = require('path');

module.exports = {
	mode: 'none',
	output: {
		path: path.resolve(__dirname, 'build/web/'),
		filename: 'commands.js',
		library: 'commands',
		libraryTarget: 'umd',
		globalObject: 'this',
	},
	optimization: {
		minimize: false
	},
};
