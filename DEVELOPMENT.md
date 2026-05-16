

```
docker build . -t caddy-fs-archives:latest
```

```
docker run -it -v ($pwd).path\Caddyfile:/etc/caddy/Caddyfile -v $HOME\Downloads:/var/www -p 8080:8080 -p 2019:2019 caddy-fs-archives 
```
