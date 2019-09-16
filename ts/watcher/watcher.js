const chokidar = require('chokidar');
const os = require('os');
const path = require('path');
const fs = require('fs');
const Crypt = require('crypto');

class Watcher {
    state = {};
    anypath = '';
    lastStrAmount = 0;
    lastStrLen = 0;
    dirName = 'anytype';
    watcher = '';
    watcherOpts = {
        ignored: /DS_Store/,
        persistent: true,
        ignoreInitial: false,
        followSymlinks: true,
        cwd: '.',
        disableGlobbing: false,
        usePolling: true,
        interval: 100,
        binaryInterval: 100,
        alwaysStat: false,
        depth: 99,
        awaitWriteFinish: {
            stabilityThreshold: 100,
            pollInterval: 100
        }
    };

    constructor () {
        this.anypath = path.resolve(os.homedir(), this.dirName);
        this.state = { tree: {} };

        if (!fs.existsSync(this.anypath)) {
            fs.mkdirSync(this.anypath);
        };

        this.watcher = chokidar.watch(this.anypath, this.watcherOpts)
            .on('add', (pth, stats) => this.addBlock(pth, stats))
            .on('change', (pth, stats) => this.changeBlock(pth, stats))
            .on('unlink', (pth, stats) => this.removeBlock(pth, stats))
            .on('addDir', (pth, stats) => this.addDoc(pth, stats))
            .on('unlinkDir', (pth, stats) => this.removeDoc(pth, stats))
            .on('error', err => console.log(`Watcher error: ${err}`));  
    };

    addBlock(pth, stats) {
        // console.log('BLOCK ADD:', pth)
        const relative = path.relative(this.anypath, pth).split('/');
        this.getFileHash(pth, (err, hash) => {
            this.put(this.state.tree, relative, hash)
            this.log(this.state.tree);            
        })
    };

    changeBlock(pth, stats) {
        // console.log('BLOCK UPDATE:', pth)
        const relative = path.relative(this.anypath, pth).split('/');
        this.getFileHash(pth, (err, hash) => {
            this.put(this.state.tree, relative, hash)
            this.log(this.state.tree);            
        })
    };

    removeBlock (pth, stats) {

    };

    addDoc(pth, stats) {
        const relative = path.relative(this.anypath, pth).split('/');
        if (relative == '') return;
        this.put(this.state.tree, relative, {})
        this.log(this.state.tree);
    };

    removeDoc (pth, stats) {

    };

    getFileHash(pth,  cb) {
        fs.readFile(pth, (err, data) => {
            return cb(null, Crypt
                .createHash('sha256')
                .update(data, 'utf8')
                .digest('hex'))
        });       
    };
      
    put (obj, path, val) {
        let stringToPath = (path) => {
            // If the path isn't a string, return it
            if (typeof path !== 'string') return path;
            // Create new array
            let output = [];
            // Split to an array with dot notation
            path.split('/').forEach((item, index) => {
                // Split to an array with bracket notation
                item.split(/\[([^}]+)\]/g).forEach((key) => {
                    // Push to the new array
                    if (key.length > 0) {
                        output.push(key);
                    }
                });
            });
            
            return output;
        };
        // Convert the path to an array if not already
        path = stringToPath(path);
        // Cache the path length and current spot in the object
        let length = path.length;
        let current = obj;
        // Loop through the path
        path.forEach((key, index) => {
            // If this is the last item in the loop, assign the value
            if (index === length -1) {
                current[key] = val;
            }
            // Otherwise, update the current place in the object
            else {
                // If the key doesn't exist, create it
                if (!current[key]) {
                    current[key] = {};
                }
                // Update the current place in the objet
                current = current[key];
            }
        });
    };

    getMethods (obj) {
        let properties = new Set()
        let currentObj = obj
        do {
          Object.getOwnPropertyNames(currentObj).map(item => properties.add(item))
        } while ((currentObj = Object.getPrototypeOf(currentObj)))
        return [...properties.keys()].filter(item => typeof obj[item] === 'function')
    };

    log(obj) {
        // process.stdout.moveCursor(-this.lastStrLen, -this.lastStrAmount);
        // let str = JSON.stringify(obj, null, ' ').split('\n')
        // this.lastStrAmount = str.length - 1
        // this.lastStrLen = str[0].length
        // process.stdout.write(JSON.stringify(obj, null, ' '));     
    };

}

const watcher = new Watcher();