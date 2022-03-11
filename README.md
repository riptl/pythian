<div align="center">
  <h1>Pythian</h1>
  <p>
    <strong>Research implementation of the Pyth data publisher</strong>
  </p>

</div>

### Summary

Pythian is a Go rewrite of the [`pythd`](https://github.com/pyth-network/pyth-client) program.

Disclaimer: **This is a research project in development. Do not deploy it on mainnet.**

### Security

The `pythian server` HTTP and WebSocket APIs should only be exposed to trusted clients.

This is because the JSON-RPC handler does not restrict request message sizes and is susceptible to slowloris attacks.

In other words, it is trivial for an attacker to exhaust all available memory.