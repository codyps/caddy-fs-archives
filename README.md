# caddy-fs-archives

A caddy filesystem for Caddy's `file_server` which allows browsing into archives
supported by github.com/mholt/archives.

This is currently a direct wrapper around `archives.DeepFS`, and so inherits
its limitations. We only go down a single level. `/foo.zip` can be browsed, but `/foo.zip/bar.zip` will return the `bar.zip` file.

The way that caddy `file_server` interacts with filesystems means one can't "download" the first archive file at all via a `file_server` using this filesystem, only browse the files inside that 
archive.

## Usage:

Include as a module:
```sh
xcaddy build --with github.com/codyps/caddy-fs-archives
```

Define a filesystem instance, and reference that instance in `file_server`:
```
# Caddyfile
{
    # "my_files" is a name you pick for this filesystem.
    # "archives" is the type of filesystem, which corresponds to the module name "caddy.fs.archives".
    filesystem my_files archives {
        # path to the root of the filesystem. This is where Caddy will look for files to serve.
        root /srv/data
    }
}

localhost:8080 {
    file_server browse {
        # "my_files" is the name of the filesystem we defined above.
        fs my_files
    }
}
```
