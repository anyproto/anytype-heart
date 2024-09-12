## Complex search filters

The majority of search filters for ObjectSearch-requests have simple format `key-condition-value`:
```json
{
     "RelationKey": "type",
     "condition": 2, // is not equal
     "value": {
        "stringValue": "ot-page"
     }
}
```

However, this format does not allow to build complex filters, that involve multiple relation keys analysis.
So we introduce complex filters that uses struct-values to build up more sophisticated requests.

**type** field defined in the struct stands for complex filters types.

### Two relation values comparison

**type** = **valueFromRelation** allows to filter out objects by comparing values of two different relations.
First relation is set to root _RelationKey_ field, second - to _relationKey_ field of the struct value.

For example, if we want to get all objects that were added to space before last modification, we can build up following filter:
```json
{
  "RelationKey": "addedDate",
  "condition": 4, // less
  "value": {
    "structValue" : {
      "fields": {
        "type": {
          "stringValue": "valueFromRelation" 
        },
        "relationKey": {
          "stringValue": "lastModifiedDate" 
        }
      }
    }
  }
}
```

All objects that has similar name and description:
```json
{
  "RelationKey": "name",
  "condition": 1, // equals
  "value": {
    "structValue": {
      "fields": {
        "type": {
          "stringValue": "valueFromRelation"
        },
        "relationKey": {
          "stringValue": "description"
        }
      }
    }
  }
}
```