package anytype

var Image = `
{
  "name": "image",
  "pin": true,
  "links": {
    "large": {
      "use": ":file",
      "pin": true,
      "plaintext": false,
      "mill": "/image/resize",
      "opts": {
        "width": "1280",
        "quality": "80"
      }
    },
    "small": {
      "use": ":file",
      "pin": true,
      "plaintext": false,
      "mill": "/image/resize",
      "opts": {
        "width": "320",
        "quality": "80"
      }
    },
    "thumb": {
      "use": ":file",
      "pin": true,
      "plaintext": false,
      "mill": "/image/resize",
      "opts": {
        "width": "100",
        "quality": "80"
      }
    },
    "exif": {
      "use": "large",
      "mill": "/image/exif"
    }
  }
}
`
