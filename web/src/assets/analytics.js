(function () {
    if (window.location.hostname !== 'anthology.bitofbytes.io') {
        return;
    }

    var script = document.createElement('script');
    script.async = true;
    script.dataset.websiteId = '7e41681c-1a39-4f02-bee6-038f5ac48df7';
    script.src = '/umami/script.js';
    document.head.appendChild(script);
})();
