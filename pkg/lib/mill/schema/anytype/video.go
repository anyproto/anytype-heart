package anytype

var Video = `
{
  "name": "video",
  "pin": true,
  "links": {
    "original": {
      "use": ":file",
      "pin": true,
      "plaintext": false,
      "mill": "/blob",
    },
    "thumbnail": {
      "use": ":file",
      "pin": true,
      "plaintext": false,
      "mill": "/video/thumbnail",
      "opts": {
        "width": "1280",
        "quality": "85"
      }
    },
    "exif": {
      "use": "large",
      "mill": "/image/exif"
    }
  }
}
`
