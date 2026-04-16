<?php
/**
 * wp-cli eval-file script: Gutenberg blocks → rendered HTML for all posts.
 * Outputs JSON array to stdout. Does NOT echo post body to terminal.
 *
 * Usage (from compose):
 *   docker compose -f compose.dev.yml exec -T client \
 *     wp eval-file /var/www/html/render-posts.php > /app/tools/rendered.json
 */

$posts = get_posts([
    'post_type'   => 'post',
    'post_status' => ['publish', 'private', 'draft'],
    'numberposts' => -1,
    'orderby'     => 'ID',
    'order'       => 'ASC',
]);

$result = [];
foreach ($posts as $p) {
    $rendered = apply_filters('the_content', $p->post_content);
    $result[] = [
        'id'     => (int) $p->ID,
        'html'   => $rendered,
        'source' => $p->post_content,
    ];
}

echo json_encode($result, JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES);
