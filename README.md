# test-discv5

### To build the image:
```
docker build -t discv5:latest .
```

### Running a bootnode:
```
docker run --rm discv5:latest
```
It should display an output similar to this:
```
enr:-Iq4QK_lBWIUHOz3glOhZZS4YukjdyJ7hia4glWzwI3rArQtcoF0PdX3enOwQ3Fn7IdUIwDHMbBjd3vARG8HOIhD8P-GAYkn9vgigmlkgnY0gmlwhKwRAAKJc2VjcDI1NmsxoQLGQTOf8RJ8cq8lemSb6NO-D4sX2F0Ja4ueEvd4Ui-Ug4N1ZHCCF3A
```
Copy this ENR or save it into a variable as it is necessary for other nodes to connect to it

### Running node(s)
```
docker run --rm discv5:latest --bootnodes=THE_BOOTNODE_ENR_GOES_HERE
```