package main

import (
	"fmt"
	"log"

	"github.com/tmsick/fish-docker-completion/cmd"
)

func main() {
	c, err := cmd.Forge("docker", nil, "")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(cmd.FishDockerCommandChainSatisfies)
	fmt.Println(cmd.FishDockerCommandChainExactlyMatches)
	fmt.Println("complete -c docker -f")
	fmt.Println("complete -c docker -l help -d 'Print usage'")
	traverse(c)
}

func traverse(c *cmd.Command) {
	fmt.Print(c.Completion())
	for _, c := range c.Subcommands {
		traverse(c)
	}
}
