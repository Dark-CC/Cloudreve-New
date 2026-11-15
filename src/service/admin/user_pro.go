package admin

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/ent"
)

type proUserFields struct {
	Credit        int64
	GroupExpires  *time.Time
	PreviousGroup *int
}

func loadProUserFields(ctx context.Context, client *ent.Client, ids []int) (map[int]*proUserFields, error) {
	res := make(map[int]*proUserFields, len(ids))
	if len(ids) == 0 {
		return res, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(
		`SELECT id, credit, group_expires, previous_group FROM users WHERE id IN (%s)`,
		strings.Join(placeholders, ","),
	)
	query, args = rebindAdminQuery(client.Driver().Dialect(), query, args)

	rows := new(entsql.Rows)
	if err := client.Driver().Query(ctx, query, args, rows); err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id            int
			credit        sql.NullInt64
			groupExpires  sql.NullTime
			previousGroup sql.NullInt64
		)
		if err := rows.Scan(&id, &credit, &groupExpires, &previousGroup); err != nil {
			return nil, err
		}

		fields := &proUserFields{Credit: credit.Int64}
		if groupExpires.Valid {
			value := groupExpires.Time
			fields.GroupExpires = &value
		}
		if previousGroup.Valid {
			value := int(previousGroup.Int64)
			fields.PreviousGroup = &value
		}
		res[id] = fields
	}

	return res, rows.Err()
}

func updateProUserFields(ctx context.Context, client *ent.Client, userID int, credit *int64,
	groupExpires *string, groupExpiresSet bool, previousGroup *int, previousGroupSet bool) error {
	sets := make([]string, 0, 3)
	args := make([]any, 0, 4)

	if credit != nil {
		sets = append(sets, "credit = ?")
		args = append(args, *credit)
	}
	if groupExpiresSet {
		sets = append(sets, "group_expires = ?")
		if groupExpires == nil || strings.TrimSpace(*groupExpires) == "" {
			args = append(args, nil)
		} else {
			value, err := time.Parse(time.RFC3339, strings.TrimSpace(*groupExpires))
			if err != nil {
				return fmt.Errorf("invalid group_expires: %w", err)
			}
			args = append(args, value)
		}
	}
	if previousGroupSet {
		sets = append(sets, "previous_group = ?")
		if previousGroup == nil || *previousGroup <= 0 {
			args = append(args, nil)
		} else {
			args = append(args, *previousGroup)
		}
	}
	if len(sets) == 0 {
		return nil
	}

	args = append(args, userID)
	query := "UPDATE users SET " + strings.Join(sets, ", ") + " WHERE id = ?"
	query, args = rebindAdminQuery(client.Driver().Dialect(), query, args)

	var result sql.Result
	if err := client.Driver().Exec(ctx, query, args, &result); err != nil {
		return err
	}
	return nil
}

func rebindAdminQuery(dialectName, query string, args []any) (string, []any) {
	if dialectName != dialect.Postgres {
		return query, args
	}

	var builder strings.Builder
	index := 0
	for _, char := range query {
		if char == '?' {
			index++
			builder.WriteString(fmt.Sprintf("$%d", index))
			continue
		}
		builder.WriteRune(char)
	}
	return builder.String(), args
}
