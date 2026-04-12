# agentchat example adapter

This example shows how a multi-service web app can map repo-specific generated local files onto the shared lane contract.

The point is not to freeze `agentchat` into this exact shape. The point is to show how a repo with:

- multiple generated local files
- a web app and a server
- stable vs dev naming concerns
- proxy-friendly hostnames

can still use a small declarative adapter.
