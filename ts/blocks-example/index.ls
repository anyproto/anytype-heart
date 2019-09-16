# Modules to control application life and create native browser window
electron = require('electron')
path = require('path')

app = electron.app
BrowserWindow = electron.BrowserWindow

# Keep a global reference of the window object, if you don't, the window will
# be closed automatically when the JavaScript object is garbage collected.
mainWindow = {}

createWindow=->
    mainWindow = new BrowserWindow do
        width: 1200
        height: 700
        # titleBarStyle: 'hidden'

    # and load the index.html of the app.
    mainWindow.loadFile('index.html')

    # Open the DevTools.
    mainWindow.webContents.openDevTools()

    # Emitted when the window is closed.
    mainWindow.on 'closed', ->
        # Dereference the window object, usually you would store windows
        # in an array if your app supports multi windows, this is the time
        # when you should delete the corresponding element.
        mainWindow = null

# This method will be called when Electron has finished
# initialization and is ready to create browser windows.
# Some APIs can only be used after this event occurs.
app.on('ready', createWindow)

# Quit when all windows are closed.
app.on 'window-all-closed', ->
    # On macOS it is common for applications and their menu bar
    # to stay active until the user quits explicitly with Cmd + Q
    if (process.platform != 'darwin') => app.quit!

app.on 'activate', ->
    # On macOS it's common to re-create a window in the app when the
    # dock icon is clicked and there are no other windows open.
    if (mainWindow == null) => createWindow!