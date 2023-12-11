### Middleware
Middleware supports following formats of images:
1. png
2. jpg
3. jpeg
4. webp 
5. ico 
6. svg 
7. gif

* Currently, if we want to get image with size > 1920, we take original image, not resized. So it leads to a case, 
when images like heic and ico are returned in original format. But in case client asked for image with size < 1920, then
heic is decoded as jpeg and ico as png.

* SVG image is saved as usual file, but we return it as image in gateway to support SVG icons for objects. Also for SVG file we create image block, 
despite the fact, that it's saved as file.

* All images are saved with following structure in DAG  
1. Original
2. Small
3. Thumb
4. Exif
5. Large

### Desktop
1. png
2. jpg
3. jpeg
4. webp 
5. svg 
6. gif

### IOS
1. png
2. jpg
3. jpeg
4. bmp
5. tiff
6. gif
7. ico
8. cur
9. xbm