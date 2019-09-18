const fs = require('fs');
const path = require('path');
const { exec } = require('child_process');
const util = require('util')

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

(async() => {
    fs.writeFileSync('scripts/protocol.proto', getCode('protocol.proto'))
    await util.promisify(exec)('protoc -I ./ scripts/protocol.proto --go_out=build/go --java_out=build/java --objc_out=build/objc')
        .then((out) => {
            if (out.stdout) console.log(out.stdout)
            if (out.stderr) console.log(out.stderr)
    });
        
    await util.promisify(exec)('pbjs -t static-module -w commonjs -o build/ts/protocol.js scripts/protocol.proto && pbts -o build/ts/protocol.d.ts build/ts/protocol.js')
        .then((out) => {
            if (out.stdout) console.log(out.stdout)
            if (out.stderr) console.log(out.stderr)
    });
    
    fs.unlinkSync('scripts/protocol.proto')
})()
