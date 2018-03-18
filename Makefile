gofiles  = download-geofabrik.go config.go download.go element.go formats.go generator.go
pkgfiles = CHANGELOG.md README.md LICENSE geofabrik.yml openstreetmap.fr.yml 
default: clean all
clean:
	go clean
	rm -rf download-geofabrik_* *.zip
gox:
	gox --output="download-geofabrik_{{.OS}}_{{.Arch}}/{{.Dir}}"
geofabrik:
	echo "Generating geofabrik.yml"
	go run $(gofiles) generate -v
osmfr:
	echo "Generating openstreetmap.fr.yml"
	go run $(gofiles) --service="openstreetmap.fr" generate -v
readme:
	cat .README.md1 > README.md
	go run $(gofiles) --help-long >> README.md 
	cat .README.md2 >> README.md
	go run $(gofiles) list --markdown >> README.md 
package: gox geofabrik osmfr 
	for i in download-geofabrik_* ;\
	do \
		  echo "Compressing $$i";\
          cp $(pkgfiles) $$i/ && cd $$i && zip -9 $$i.zip download-geofabrik* $(pkgfiles) && mv $$i.zip ../ && cd ..;\
        done

all: package
