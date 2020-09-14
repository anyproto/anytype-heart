{
  'targets': [
    {
      'target_name': 'addon',
      'sources': [ 'addon.c' ],
      "libraries": ["<!(pwd)/lib.a" ],
      "conditions": [
        [ "OS=='linux'", {
            "ldflags": [ "-Wl,-Bsymbolic" ]
        }]
      ]
    }
  ]
}
