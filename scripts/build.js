const fs = require('fs');
const path = require('path');
const {exec} = require('child_process');

const getCode = (filename) => {
    let fileRoot = fs.readFileSync( filename, 'utf8')
    fileRoot.split('\n')
        .filter(el => /import \"[\S]+.proto\"\;/.test(el))
        .map(el => /"([\S]+.proto)"/.exec(el))
        .map(el => el[1]) 
        .map(f => {
            let file = fs.readFileSync(f, 'utf8')
            fileRoot += file.replace('syntax="proto3";', '')
        })

    return fileRoot.replace(/import \"[\S]+.proto\"\;/, '')
}

fs.writeFileSync('scripts/protocol.proto', getCode('protocol.proto'))
exec('protoc -I ./ scripts/protocol.proto --go_out=build/go --java_out=build/java --objc_out=build/objc', { shell: true }, (stdout, err) => {
    if (err) console.log(err)
});

exec('pbjs -t static-module -w commonjs -o build/ts/protocol.js scripts/protocol.proto && pbts -o build/ts/protocol.d.ts build/ts/protocol.js', { shell: true }, (stdout, err) => {
    if (err) console.log(err)
});

fs.unlinkSync('scripts/protocol.proto')