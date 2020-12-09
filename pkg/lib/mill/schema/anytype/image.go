package anytype

var Image = `
{
  "name": "image",
  "pin": true,
  "links": {
    "original": {
	  "use": ":file",
	  "pin": true,
	  "plaintext": false,
	  "mill": "/blob"
    },
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
      "use": ":file",
      "mill": "/image/exif"
    }
  }
}
`
