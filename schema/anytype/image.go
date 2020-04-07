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
        "width": "1920",
        "quality": "85"
      }
    },
    "small": {
      "use": ":file",
      "pin": true,
      "plaintext": false,
      "mill": "/image/resize",
      "opts": {
        "width": "320",
        "quality": "85"
      }
    },
    "thumb": {
      "use": ":file",
      "pin": true,
      "plaintext": false,
      "mill": "/image/resize",
      "opts": {
        "width": "100",
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
