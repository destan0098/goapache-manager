package apache

import "testing"

func TestParseVHosts(t *testing.T) {
	output := `VirtualHost configuration:
*:80 is a NameVirtualHost
         default server localhost (/etc/apache2/sites-enabled/000-default.conf:1)
         port 80 namevhost localhost (/etc/apache2/sites-enabled/000-default.conf:1)
         port 80 namevhost test.com (/etc/apache2/sites-enabled/000-default.conf:11)`

	result := ParseVHosts(output)

	if len(result) != 2 {
		t.Fatalf("expected 2 vhosts, got %d", len(result))
	}

	if result[0].Domain != "localhost" {
		t.Errorf("expected localhost, got %s", result[0].Domain)
	}

	if result[1].Domain != "test.com" {
		t.Errorf("expected test.com, got %s", result[1].Domain)
	}
}
