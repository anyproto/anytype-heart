var path = require('path');

module.exports = {
	mode: 'none',
	output: {
		path: path.resolve(__dirname, 'build/web/'),
		filename: 'commands.pb.js'
	},
	optimization: {
		minimize: false
	},
};
