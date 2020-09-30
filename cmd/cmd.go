package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

const FishDockerCommandChainSatisfies = `function __fish_docker_command_chain_satisfies
    set -l cmd (commandline -poc)
    if test (count $cmd) -lt (count $argv)
        return 1
    end
    for i in (seq (count $argv))
        if test $cmd[$i] != $argv[$i]
            return 1
        end
    end
    return 0
end`

const FishDockerCommandChainExactlyMatches = `function __fish_docker_command_chain_exactly_matches
    if not __fish_docker_command_chain_satisfies $argv
        return 1
    end
    set -l cmd (commandline -poc)
    if test (count $cmd) -eq (count $argv)
        return 0
    end
    string match -q -r '^--?\w+' -- $cmd[(math 1 + (count $argv))]
end`

type Command struct {
	Chain       []string
	Desc        string
	Arguments   int
	Options     []*Option
	Subcommands []*Command
	helpMessage []byte
}

type Option struct {
	Desc  string
	Long  string
	Short string
}

type Argument struct {
	Type    string
	Command string
}

const (
	ArgumentNumberDockerConfig = 1 << iota
	ArgumentNumberDockerContainer
	ArgumentNumberDockerImage
	ArgumentNumberDockerNetwork
	ArgumentNumberDockerNode
	ArgumentNumberDockerPlugin
	ArgumentNumberDockerSecret
	ArgumentNumberDockerService
	ArgumentNumberDockerStack
	ArgumentNumberDockerVolume
	ArgumentNumberFile
)

var Arguments = map[int]Argument{
	ArgumentNumberDockerConfig:    {"Config", "(docker config ls)"},
	ArgumentNumberDockerContainer: {"Container", "(docker container ls --all --format='{{.Names}}')"},
	ArgumentNumberDockerImage:     {"Image", "(docker image ls --format='{{.Repository}}:{{.Tag}}')"},
	ArgumentNumberDockerNetwork:   {"Network", "(docker network ls --format='{{.Name}}')"},
	ArgumentNumberDockerNode:      {"Node", "(docker node ls --format='{{.Name}}')"},
	ArgumentNumberDockerPlugin:    {"Plugin", "(docker plugin ls --format='{{.Name}}')"},
	ArgumentNumberDockerSecret:    {"Secret", "(docker secret ls --format='{{.Name}}')"},
	ArgumentNumberDockerService:   {"Service", "(docker service ls --format='{{.Name}}')"},
	ArgumentNumberDockerStack:     {"Stack", "(docker stack ls --format='{{.Name}}')"},
	ArgumentNumberDockerVolume:    {"Volume", "(docker volume ls --format='{{.Name}}')"},
	ArgumentNumberFile:            {"", "(ls)"},
}

func Forge(cmd string, chain []string, desc string) (c *Command, err error) {
	arg := make([]string, 0)
	arg = append(arg, chain...)
	arg = append(arg, cmd)
	arg = append(arg, "--help")
	msg, err := exec.Command(arg[0], arg[1:]...).Output()
	if err != nil {
		return
	}
	arg = arg[:len(arg)-1] // Remove "--help"
	c = &Command{}
	c.Chain = arg
	c.Desc = desc
	c.helpMessage = msg
	c.setArgument()
	if err = c.setOptions(); err != nil {
		return
	}
	if err = c.setSubcommands(); err != nil {
		return
	}
	return
}

func (c *Command) ChainString() string {
	return strings.Join(c.Chain, " ")
}

func (c *Command) Completion() string {
	var s string
	for k, v := range Arguments {
		if k&c.Arguments == 0 {
			continue
		}
		s += fmt.Sprintf("complete -c docker -n '__fish_docker_command_chain_exactly_matches %s' -a %q -d %q\n", c.ChainString(), v.Command, v.Type)
	}
	for _, sc := range c.Subcommands {
		s += fmt.Sprintf("complete -c docker -n '__fish_docker_command_chain_exactly_matches %s' -a %s -d %q\n", c.ChainString(), sc.Chain[len(sc.Chain)-1], sc.Desc)
	}
	for _, opt := range c.Options {
		s += fmt.Sprintf("complete -c docker -n '__fish_docker_command_chain_exactly_matches %s'", c.ChainString())
		if opt.Short != "" {
			s += fmt.Sprintf(" -s %s", opt.Short)
		}
		if opt.Long != "" {
			s += fmt.Sprintf(" -l %s", opt.Long)
		}
		s += fmt.Sprintf(" -d %q\n", opt.Desc)
	}
	return s
}

func (c *Command) setArgument() {
	linesMap := c.scanHelpMessage("Usage:")
	uppercasedPattern := regexp.MustCompile(`[A-Z_]+`)
	lowercasedPattern := regexp.MustCompile(`[a-z_]+`)
	var number int
	var lines []string
	for _, v := range linesMap {
		lines = v
		break
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		trimmed := strings.TrimPrefix(line, c.ChainString())
		if trimmed == line {
			return
		}
		for _, match := range uppercasedPattern.FindAllString(trimmed, -1) {
			switch match {
			case "CONFIG":
				number |= ArgumentNumberDockerConfig
			case "CONTAINER":
				number |= ArgumentNumberDockerContainer
			case "IMAGE", "SOURCE_IMAGE", "TARGET_IMAGE":
				number |= ArgumentNumberDockerImage
			case "NETWORK":
				number |= ArgumentNumberDockerNetwork
			case "NODE":
				number |= ArgumentNumberDockerNode
			case "PLUGIN":
				number |= ArgumentNumberDockerPlugin
			case "SECRET":
				number |= ArgumentNumberDockerSecret
			case "SERVICE":
				number |= ArgumentNumberDockerService
			case "STACK":
				number |= ArgumentNumberDockerStack
			case "VOLUME":
				number |= ArgumentNumberDockerVolume
			case "KEY_FILE":
				number |= ArgumentNumberFile
			}
		}
		for _, match := range lowercasedPattern.FindAllString(trimmed, -1) {
			switch match {
			case "file":
				number |= ArgumentNumberFile
			}
		}
	}
	c.Arguments = number
}

func (c *Command) setOptions() error {
	linesMap := c.scanHelpMessage("Options:")
	pairsMap := splitDescription(linesMap)
	pairs := make([][2]string, 0)
	for _, v := range pairsMap {
		pairs = append(pairs, v...)
	}
	for _, pair := range pairs {
		opt := strings.TrimSpace(pair[0])
		desc := strings.TrimSpace(pair[1])
		var long, short string
		for _, flag := range strings.Split(opt, " ") {
			flag = strings.TrimSuffix(flag, ",")
			if strings.HasPrefix(flag, "--") {
				long = strings.TrimPrefix(flag, "--")
			} else if strings.HasPrefix(flag, "-") {
				short = strings.TrimPrefix(flag, "-")
			}
		}
		if opt == "" {
			if len(c.Options) == 0 {
				return fmt.Errorf("failed to options for %q", c.ChainString())
			}
			c.Options[len(c.Options)-1].Desc += " " + desc
		} else {
			c.Options = append(c.Options, &Option{
				Desc:  desc,
				Long:  long,
				Short: short,
			})
		}
	}
	return nil
}

func (c *Command) setSubcommands() error {
	linesMap := c.scanHelpMessage("Commands:", "Available Commands:", "Management Commands:")
	pairsMap := splitDescription(linesMap)
	pairs := make([][2]string, 0)
	for _, v := range pairsMap {
		pairs = append(pairs, v...)
	}
	for _, pair := range pairs {
		cmd := strings.TrimSpace(pair[0])
		desc := strings.TrimSpace(pair[1])
		if cmd == "" {
			if len(c.Subcommands) == 0 {
				return fmt.Errorf("failed to subcommands of %q", c.ChainString())
			}
			c.Subcommands[len(c.Subcommands)-1].Desc += " " + desc
		} else {
			subcommand, err := Forge(cmd, c.Chain, desc)
			if err != nil {
				return err
			}
			c.Subcommands = append(c.Subcommands, subcommand)
		}
	}
	return nil
}

func (c *Command) scanHelpMessage(markers ...string) map[string][]string {
	blankPattern := regexp.MustCompile(`^\s*$`)
	currentMarker := ""
	linesMap := make(map[string][]string)
	scanner := bufio.NewScanner(bytes.NewReader(c.helpMessage))
	for scanner.Scan() {
		line := scanner.Text()
		var hitMarker bool
		if blankPattern.MatchString(line) {
			currentMarker = ""
		} else {
			for _, marker := range markers {
				if !strings.HasPrefix(line, marker) {
					continue
				}
				hitMarker = true
				currentMarker = marker
				trailer := strings.TrimPrefix(line, marker)
				if strings.TrimSpace(trailer) != "" {
					linesMap[currentMarker] = append(linesMap[currentMarker], trailer)
				}
				break
			}
		}
		if hitMarker || currentMarker == "" {
			continue
		}
		linesMap[currentMarker] = append(linesMap[currentMarker], line)
	}
	return linesMap
}

func splitDescription(linesMap map[string][]string) map[string][][2]string {
	descHeadPattern := regexp.MustCompile(`\s[A-Z]`)
	pairsMap := make(map[string][][2]string)
	for key, lines := range linesMap {
		descHeadIndices := make(map[int]int)
		for _, line := range lines {
			for _, tuple := range descHeadPattern.FindAllIndex([]byte(line), -1) {
				descHeadIndices[tuple[0]+1]++
			}
		}
		var descHeadIndex, votes int
		for k, v := range descHeadIndices {
			if v > votes {
				votes = v
				descHeadIndex = k
			}
		}
		for _, line := range lines {
			pairsMap[key] = append(pairsMap[key], [...]string{line[:descHeadIndex], line[descHeadIndex:]})
		}
	}
	return pairsMap
}
