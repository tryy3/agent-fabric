I am looking to create a new larger feature-set.

Short version is that I want to add more project specific tools including a lightweight codepen-like workspace.

## Project
A project should be a collection of agentic threads/workflows with project-specific settings.
This allows us to create environments for project isolation.

For example with docker we could allows per project docker container and volume, giving the agents isolation between projects but a shared environment to work on overtime.

Similarly in the future when we start adding memories, contexts, other tools we could decide on a per-project basis how they should be configured.
It is also possible we can have global and per-project settings allowing certain things to be more global compared to others.

With environments like SSH (and I suppose docker) we could decide if projects need isolation or if they might want to share between projects.

## CodePen-like Workspace
The idea is to create a more lightweight version of codepen or editors like vscode.

The idea is to add
* file explorer - to view files in the project environment we are in
* File editor - editor with syntax highlighting and ability to save changes
* File preview - if HTML/JS it should attempt to render this, if image it should display, then we can add like music player and other things.

I also would like to explore the ability to download and upload the project to different providers, for example a S3 bucket, Github or similar services.
This would allow the user to both download their whole project, but also upload it and share it with others.

## Other Ideas
I would also like to explore versioning in some way, when you iterate with the agentic it would be nice to be able to go through different versions, but I am not to sure how this should work and what technology it should be behind it.


## Final Thoughts
While I might get carried away I think it is good to remind myself that with the architecture I have I will have the ability to use different clients for different use cases.
In the flutter client the main focus will be the chat interface with something like this on top of it to help me prototype smaller projects.
In the event that I want to do something larger it might be better to do it through a proper editor like Zed which supports ACP. The idea being that I can code locally in that case but still use my CP platform to get access to things like memories, contexts, tools, etc.
