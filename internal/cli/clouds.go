package cli

// Every supported cloud is linked in here and registers itself with
// internal/cloud from its init(). To add a cloud, create
// internal/provider/<name> (see .claude/skills/add-cloud) and add one line.
import (
	_ "github.com/SVMadhavaReddy/cloudssh/internal/provider/aws"
)
