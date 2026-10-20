# Validate the API base URL before it enters JavaScript or the nginx CSP.
# Deliberately accept only root-relative paths and HTTP(S) DNS/IPv4 URLs;
# query strings, fragments, credentials, IPv6 literals and protocol-relative
# URLs are not API base URLs supported by this container.
function invalid() {
    print "Invalid NG_APP_API_URL: expected /path or an http(s) DNS/IPv4 URL, without credentials, query or fragment" > "/dev/stderr"
    exit 1
}
BEGIN {
    url = ENVIRON["NG_APP_API_URL"]
    if (length(url) > 2048 || url ~ /[^A-Za-z0-9._~:\/%+-]/) invalid()
    escaped = url
    gsub(/%[0-9A-Fa-f][0-9A-Fa-f]/, "", escaped)
    if (escaped ~ /%/) invalid()
    if (url ~ /^\//) {
        if (url ~ /^\/\//) invalid()
        print ""
        exit
    }
    if (url !~ /^https?:\/\//) invalid()
    scheme = url
    sub(/:\/\/.*/, "", scheme)
    authority = url
    sub(/^https?:\/\//, "", authority)
    sub(/\/.*/, "", authority)
    host = authority
    sub(/:.*/, "", host)
    if (host == "" || length(host) > 253 || host ~ /[^A-Za-z0-9.-]/) invalid()
    n = split(host, labels, ".")
    for (i = 1; i <= n; i++) {
        if (length(labels[i]) < 1 || length(labels[i]) > 63 ||
            labels[i] ~ /^-/ || labels[i] ~ /-$/) invalid()
    }
    # Browsers interpret numeric hostnames as IPv4, including non-decimal
    # forms. Only accept the unambiguous dotted-decimal representation.
    if (labels[n] ~ /^[0-9]+$/ || labels[n] ~ /^0[xX][0-9A-Fa-f]+$/) {
        if (n != 4) invalid()
        for (i = 1; i <= n; i++) {
            if (labels[i] !~ /^[0-9]+$/ || labels[i] + 0 > 255 ||
                (length(labels[i]) > 1 && labels[i] ~ /^0/)) invalid()
        }
    }
    port = ""
    if (authority != host) {
        port = substr(authority, length(host) + 2)
        if (port !~ /^[0-9]+$/ || length(port) > 5 || port + 0 < 1 || port + 0 > 65535) invalid()
        port = sprintf(":%d", port)
        if ((scheme == "http" && port == ":80") || (scheme == "https" && port == ":443")) port = ""
    }
    print scheme "://" tolower(host) port
}
